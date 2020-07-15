package main

import (
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/eks"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ec2"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		/*
		 * Construct a slug which references another stack to use in our stack reference
		 */
		slug := fmt.Sprintf("jaxxstorm/vpc.go/%v", ctx.Stack())
		vpc, err := pulumi.NewStackReference(ctx, slug, nil)
		if err != nil {
			fmt.Errorf("Error getting vpc stack reference: %w", err)
		}

		/*
		 * Add the IAM Role and policies to use EKS
		 */
		eksAssumeRolePolicyJSON, _ := json.Marshal(map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []interface{}{
				map[string]interface{}{
					"Action": "sts:AssumeRole",
					"Principal": map[string]interface{}{
						"Service": []interface{}{
							"eks.amazonaws.com",
						},
					},
					"Effect": "Allow",
				},
			},
		})
		eksRole, err := iam.NewRole(ctx, "eks-iam-eksRole", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(eksAssumeRolePolicyJSON),
		})
		if err != nil {
			return err
		}
		eksPolicies := []string{
			"arn:aws:iam::aws:policy/AmazonEKSServicePolicy",
			"arn:aws:iam::aws:policy/AmazonEKSClusterPolicy",
		}
		for i, eksPolicy := range eksPolicies {
			_, err := iam.NewRolePolicyAttachment(ctx, fmt.Sprintf("rpa-%d", i), &iam.RolePolicyAttachmentArgs{
				PolicyArn: pulumi.String(eksPolicy),
				Role:      eksRole.Name,
			}, pulumi.Parent(eksRole))
			if err != nil {
				return err
			}
		}

		nodeGroupAssumeRolePolicyJSON, _ := json.Marshal(map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []interface{}{
				map[string]interface{}{
					"Action": "sts:AssumeRole",
					"Principal": map[string]interface{}{
						"Service": []interface{}{
							"ec2.amazonaws.com",
						},
					},
					"Effect": "Allow",
				},
			},
		})
		// Create the EC2 NodeGroup Role
		nodeGroupRole, err := iam.NewRole(ctx, "nodegroup-iam-role", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(nodeGroupAssumeRolePolicyJSON),
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		})
		if err != nil {
			return err
		}
		nodeGroupPolicies := []string{
			"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
			"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
			"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
		}

		for i, nodeGroupPolicy := range nodeGroupPolicies {
			_, err := iam.NewRolePolicyAttachment(ctx, fmt.Sprintf("ngpa-%d", i), &iam.RolePolicyAttachmentArgs{
				Role:      nodeGroupRole.Name,
				PolicyArn: pulumi.String(nodeGroupPolicy),
			}, pulumi.Parent(nodeGroupRole))
			if err != nil {
				return err
			}
		}

		// Create a Security Group that we can use to actually connect to our cluster
		clusterSg, err := ec2.NewSecurityGroup(ctx, "cluster-sg", &ec2.SecurityGroupArgs{
			VpcId: vpc.GetStringOutput(pulumi.String("id")),
			Egress: ec2.SecurityGroupEgressArray{
				ec2.SecurityGroupEgressArgs{
					Protocol:   pulumi.String("-1"),
					FromPort:   pulumi.Int(0),
					ToPort:     pulumi.Int(0),
					CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				},
			},
			Ingress: ec2.SecurityGroupIngressArray{
				ec2.SecurityGroupIngressArgs{
					Protocol:   pulumi.String("tcp"),
					FromPort:   pulumi.Int(80),
					ToPort:     pulumi.Int(80),
					CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				},
			},
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		})
		if err != nil {
			return err
		}

		// Create EKS Cluster
		eksCluster, err := eks.NewCluster(ctx, "eks-cluster", &eks.ClusterArgs{
			Name:    pulumi.String("lbriggs"),
			RoleArn: pulumi.StringInput(eksRole.Arn),
			VpcConfig: &eks.ClusterVpcConfigArgs{
				PublicAccessCidrs: pulumi.StringArray{
					pulumi.String("0.0.0.0/0"),
				},
				SecurityGroupIds: pulumi.StringArray{
					clusterSg.ID().ToStringOutput(),
				},
				SubnetIds: pulumi.StringArrayOutput(vpc.GetOutput(pulumi.String("privateSubnets"))),
			},
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		})
		if err != nil {
			return err
		}

		_, err = eks.NewNodeGroup(ctx, "node-group", &eks.NodeGroupArgs{
			ClusterName:   eksCluster.Name,
			NodeGroupName: pulumi.String("node-group"),
			NodeRoleArn:   pulumi.StringInput(nodeGroupRole.Arn),
			SubnetIds:     pulumi.StringArrayOutput(vpc.GetOutput(pulumi.String("privateSubnets"))),
			ScalingConfig: &eks.NodeGroupScalingConfigArgs{
				DesiredSize: pulumi.Int(2),
				MaxSize:     pulumi.Int(2),
				MinSize:     pulumi.Int(1),
			},
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		}, pulumi.Parent(eksCluster))
		if err != nil {
			return err
		}

		kubeConfig, err := generateKubeconfig(eksCluster.Endpoint, eksCluster.CertificateAuthority.Data().Elem(), eksCluster.Name)

		if err != nil {
			return err
		}

		ctx.Export("kubeconfig", kubeConfig)

		return nil
	})
}
