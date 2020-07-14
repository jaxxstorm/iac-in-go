package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/pulumi/pulumi/sdk/v2/go/pulumi/config"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ssm"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/autoscaling"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		config := config.New(ctx, "")
		tailScaleHostKey := config.Require("tailScaleHostKey")

		/*
		 * Grab the vpc cluster stack outputs
		 */
		vpcSlug := fmt.Sprintf("jaxxstorm/vpc.go/%v", ctx.Stack())
		vpc, err := pulumi.NewStackReference(ctx, vpcSlug, nil)
		if err != nil {
			return fmt.Errorf("Error getting vpc stack reference: %w", err)
		}

		/*
		 * Store the tailscale auth key in AWS SSM
		 */

		tailScaleKeyParameter, err := ssm.NewParameter(ctx, "tailscale-auth-key", &ssm.ParameterArgs{
			Name:  pulumi.String("tailscale-auth-key"),
			Type:  pulumi.String("SecureString"),
			Value: pulumi.String(tailScaleHostKey),
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		})

		/*
		 * IAM policy principal
		 */
		assumeRolePolicyJSON, _ := json.Marshal(map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []interface{}{
				map[string]interface{}{
					"Action": "sts:AssumeRole",
					"Principal": map[string]interface{}{
						"Service": []interface{}{
							"ec2.amazonaws.com",
							"ssm.amazonaws.com",
						},
					},
					"Effect": "Allow",
				},
			},
		})

		bastionSSMPolicyJSON := tailScaleKeyParameter.Arn.ApplyT(func(arn string) (string, error) {
			policyJSON, err := json.Marshal(map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []interface{}{
					map[string]interface{}{
						"Action": []string{
							"ssm:GetParameters",
						},
						"Effect": "Allow",
						"Resource": []string{
							arn,
						},
					},
					map[string]interface{}{
						"Action": []string{
							"ssm:DescribeParameters",
						},
						"Effect":   "Allow",
						"Resource": "*",
					},
				},
			})
			if err != nil {
				return "", err
			}
			return string(policyJSON), nil
		})

		/*
		 * Create the IAM role that allows talking to EC2 and SSM
		 */
		bastionIAMRole, err := iam.NewRole(ctx, "bastion", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(assumeRolePolicyJSON),
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		})
		if err != nil {
			return err
		}

		/*
		 * Attach the policy to the role that allows running on ECS
		 */
		_, err = iam.NewRolePolicyAttachment(ctx, "ssm-managed-instance-policy", &iam.RolePolicyAttachmentArgs{
			Role:      bastionIAMRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"),
		}, pulumi.Parent(bastionIAMRole))
		if err != nil {
			return err
		}

		/*
		 * Attach a policy that allows the instances to retrieve
		 * parameters from the parameter store
		 */
		ssmPolicy, err := iam.NewPolicy(ctx, "bastion-ssm-access", &iam.PolicyArgs{
			Policy: bastionSSMPolicyJSON,
		}, pulumi.Parent(bastionIAMRole))
		if err != nil {
			return err
		}

		_, err = iam.NewRolePolicyAttachment(ctx, "ssm-get-parameters", &iam.RolePolicyAttachmentArgs{
			Role:      bastionIAMRole.Name,
			PolicyArn: ssmPolicy.Arn,
		}, pulumi.Parent(ssmPolicy))

		/*
		 * Create an IAM instance profile to assign to the ASG
		 */
		bastionIAMInstanceProfile, err := iam.NewInstanceProfile(ctx, "bastion", &iam.InstanceProfileArgs{
			Role: bastionIAMRole.Name,
		}, pulumi.Parent(bastionIAMRole))
		if err != nil {
			return err
		}

		/*
		 * Create a security group for the bastion traffic
		 */
		bastionSecurityGroup, err := ec2.NewSecurityGroup(ctx, "bastion", &ec2.SecurityGroupArgs{
			Description: pulumi.String("Allow egress traffic for bastion host"),
			VpcId:       vpc.GetStringOutput(pulumi.String("id")),
			Ingress: &ec2.SecurityGroupIngressArray{
				&ec2.SecurityGroupIngressArgs{
					Protocol: pulumi.String("icmp"),
					FromPort: pulumi.Int(0),
					ToPort:   pulumi.Int(0),
					CidrBlocks: pulumi.StringArray{
						pulumi.String("0.0.0.0/0"),
					},
				},
				&ec2.SecurityGroupIngressArgs{
					Protocol: pulumi.String("tcp"),
					FromPort: pulumi.Int(22),
					ToPort:   pulumi.Int(22),
					CidrBlocks: pulumi.StringArray{
						pulumi.String("0.0.0.0/0"),
					},
				},
			},
			Egress: &ec2.SecurityGroupEgressArray{
				&ec2.SecurityGroupEgressArgs{
					Protocol: pulumi.String("-1"),
					FromPort: pulumi.Int(0),
					ToPort:   pulumi.Int(0),
					CidrBlocks: pulumi.StringArray{
						pulumi.String("0.0.0.0/0"),
					},
				},
			},
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		})
		if err != nil {
			return err
		}

		/*
		 * Retrieve the AMI
		 */
		mostRecent := true
		ami, err := aws.GetAmi(ctx, &aws.GetAmiArgs{
			Filters: []aws.GetAmiFilter{
				{
					Name:   "owner-alias",
					Values: []string{"amazon"},
				},
				{
					Name:   "name",
					Values: []string{"amzn2-ami-hvm*"},
				},
			},
			Owners:     []string{"amazon"},
			MostRecent: &mostRecent,
		})
		if err != nil {
			return err
		}

		userData, err := ioutil.ReadFile("userdata.init")
		if err != nil {
			return err
		}

		/*
		 * Create an AWS Launch Configuration
		 */
		bastionLaunchConfiguration, err := ec2.NewLaunchConfiguration(ctx, "bastion", &ec2.LaunchConfigurationArgs{
			InstanceType: pulumi.String("t2.micro"),
			SecurityGroups: pulumi.StringArray{
				bastionSecurityGroup.ID(),
			},
			AssociatePublicIpAddress: pulumi.Bool(false),
			ImageId:                  pulumi.String(ami.Id),
			IamInstanceProfile:       bastionIAMInstanceProfile.ID(),
			UserData:                 pulumi.String(base64.StdEncoding.EncodeToString(userData)),
			KeyName:                  pulumi.String("lbriggs"),
		})
		if err != nil {
			return err
		}

		bastionAutoScalingGroup, err := autoscaling.NewGroup(ctx, "bastion", &autoscaling.GroupArgs{
			LaunchConfiguration:    bastionLaunchConfiguration.ID(),
			MaxSize:                pulumi.Int(1),
			MinSize:                pulumi.Int(1),
			HealthCheckType:        pulumi.String("EC2"),
			HealthCheckGracePeriod: pulumi.Int(30),
			VpcZoneIdentifiers:     pulumi.StringArrayOutput(vpc.GetOutput(pulumi.String("privateSubnets"))),
			Tags: &autoscaling.GroupTagArray{
				&autoscaling.GroupTagArgs{
					Key:               pulumi.String("Owner"),
					Value:             pulumi.String("lbriggs"),
					PropagateAtLaunch: pulumi.Bool(true),
				},
				&autoscaling.GroupTagArgs{
					Key:               pulumi.String("Name"),
					Value:             pulumi.String("lbriggs-bastion"),
					PropagateAtLaunch: pulumi.Bool(true),
				},
			},
		}, pulumi.Parent(bastionLaunchConfiguration))

		ctx.Export("autoScalingGroupName", bastionAutoScalingGroup.Name)
		return nil
	})
}
