package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/lb"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		// grab the vpc cluster stack outputs
		vpcSlug := fmt.Sprintf("jaxxstorm/vpc.go/%v", ctx.Stack())
		vpc, err := pulumi.NewStackReference(ctx, vpcSlug, nil)
		if err != nil {
			return fmt.Errorf("Error getting vpc stack reference: %w", err)
		}

		webSecurityGroup, err := ec2.NewSecurityGroup(ctx, "web", &ec2.SecurityGroupArgs{
			VpcId:       vpc.GetStringOutput((pulumi.String("id"))),
			Description: pulumi.String("Web security for ALB"),
			Ingress: &ec2.SecurityGroupIngressArray{
				&ec2.SecurityGroupIngressArgs{
					Protocol: pulumi.String("tcp"),
					FromPort: pulumi.Int(80),
					ToPort:   pulumi.Int(80),
					CidrBlocks: pulumi.StringArray{
						pulumi.String("0.0.0.0/0"),
					},
				},
				&ec2.SecurityGroupIngressArgs{
					Protocol: pulumi.String("tcp"),
					FromPort: pulumi.Int(443),
					ToPort:   pulumi.Int(443),
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
		})

		alb, err := lb.NewLoadBalancer(ctx, "web", &lb.LoadBalancerArgs{
			SecurityGroups: pulumi.StringArray{
				webSecurityGroup.ID(),
			},
			Subnets: pulumi.StringArrayOutput(vpc.GetOutput(pulumi.String("privateSubnets"))),
		})

		httpListener, err := lb.NewListener(ctx, "http", &lb.ListenerArgs{
			LoadBalancerArn: alb.Arn,
			Port:            pulumi.Int(80),
			DefaultActions: &lb.ListenerDefaultActionArray{
				&lb.ListenerDefaultActionArgs{
					Type: pulumi.String("redirect"),
					Redirect: &lb.ListenerDefaultActionRedirectArgs{
						Port:       pulumi.String("443"),
						Protocol:   pulumi.String("HTTPS"),
						StatusCode: pulumi.String("HTTP_301"),
					},
				},
			},
		}, pulumi.Parent(alb))

		httpsListener, err := lb.NewListener(ctx, "https", &lb.ListenerArgs{
			LoadBalancerArn: alb.Arn,
			Port:            pulumi.Int(443),
			DefaultActions: &lb.ListenerDefaultActionArray{
				&lb.ListenerDefaultActionArgs{
					Type: pulumi.String("fixed-response"),
					FixedResponse: &lb.ListenerDefaultActionFixedResponseArgs{
						ContentType: pulumi.String("text/plain"),
						MessageBody: pulumi.String("You seem to be lost"),
						StatusCode:  pulumi.String("200"),
					},
				},
			},
		}, pulumi.Parent(alb))

		ctx.Export("arn", alb.Arn)
		ctx.Export("httpListenerArn", httpListener.Arn)
		ctx.Export("httpsListenerArn", httpsListener.Arn)

		return nil
	})
}
