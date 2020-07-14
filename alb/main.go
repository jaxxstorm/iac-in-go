package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/lb"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		/*
		 * Grab the VPC stack outputs
		 * FIXME: make these configurable
		 */
		vpcSlug := fmt.Sprintf("jaxxstorm/vpc.go/%v", ctx.Stack())
		vpc, err := pulumi.NewStackReference(ctx, vpcSlug, nil)
		if err != nil {
			return fmt.Errorf("Error getting vpc stack reference: %w", err)
		}

		/*
		 * Create a security group for the ALB that allows
		 * HTTPS & HTTP traffic
		 */
		webSecurityGroup, err := ec2.NewSecurityGroup(ctx, "web", &ec2.SecurityGroupArgs{
			VpcId:       vpc.GetStringOutput(pulumi.String("id")),
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

		/*
		 * Create an ALB
		 * We use the public subnets from the VPC stack as an input
		 */
		alb, err := lb.NewLoadBalancer(ctx, "web", &lb.LoadBalancerArgs{
			SecurityGroups: pulumi.StringArray{
				webSecurityGroup.ID(),
			},
			Subnets: pulumi.StringArrayOutput(vpc.GetOutput(pulumi.String("publicSubnets"))),
		})

		/*
		 * Add a HTTP listener to the ALB
		 * This always redirects to HTTPs as a 301
		 */
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

		/*
		 * Create the HTTPS listener, with a default fixed
		 * response if the host header isn't specified
		 */
		httpsListener, err := lb.NewListener(ctx, "https", &lb.ListenerArgs{
			LoadBalancerArn: alb.Arn,
			Port:            pulumi.Int(443),
			Protocol:        pulumi.String("HTTPS"),
			CertificateArn:  pulumi.String("arn:aws:acm:us-west-2:616138583583:certificate/bb362d39-6233-415b-8270-b459128f2cbe"),
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

		/*
		 * Export some values for other stacks
		 */
		ctx.Export("arn", alb.Arn)
		ctx.Export("dnsName", alb.DnsName)
		ctx.Export("httpListenerArn", httpListener.Arn)
		ctx.Export("httpsListenerArn", httpsListener.Arn)

		return nil
	})
}
