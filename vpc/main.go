package main

import (
	vpc "github.com/jaxxstorm/pulumi-aws-vpc/go"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		/*
		 * Create a VPC
		 * this uses a component resource, which is defined on line 4
		 */
		awsVpc, err := vpc.NewVpc(ctx, "lbriggs", vpc.Args{
			BaseCidr:    "172.1.0.0/16",
			Description: "lbriggs-vpc",
			ZoneName:    "aws.lbrlabs.com",
			AvailabilityZoneNames: pulumi.StringArray{
				pulumi.String("us-west-2a"),
				pulumi.String("us-west-2b"),
				pulumi.String("us-west-2c"),
			},
			BaseTags: pulumi.StringMap{
				"Owner": pulumi.String("lbriggs"),
			},
			Endpoints: vpc.Endpoints{
				S3:       true,
				DynamoDB: true,
			},
		})

		if err != nil {
			return err
		}

		ctx.Export("id", awsVpc.ID)
		ctx.Export("arn", awsVpc.Arn)
		ctx.Export("publicSubnets", awsVpc.PublicSubnets)
		ctx.Export("privateSubnets", awsVpc.PrivateSubnets)
		return nil
	})
}
