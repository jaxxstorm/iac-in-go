package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/rds"
	"github.com/pulumi/pulumi-random/sdk/v2/go/random"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		// we construct a slug which references another stack to use in our stack reference
		slug := fmt.Sprintf("jaxxstorm/vpc.go/%v", ctx.Stack())
		vpc, err := pulumi.NewStackReference(ctx, slug, nil)
		if err != nil {
			fmt.Errorf("Error getting vpc stack reference: %w", err)
		}

		// we have to do some type casting here to ensure the subnets are in the right format
		// we need for the subnet group
		subnets := pulumi.StringArrayOutput(vpc.GetOutput(pulumi.String("privateSubnets")))

		// create an RDS subnet group which can be used by the database
		_, err = rds.NewSubnetGroup(ctx, "db-subnet-group", &rds.SubnetGroupArgs{
			SubnetIds: subnets,
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		})
		if err != nil {
			return err
		}

		// generate a random password using the random provider
		dbPassword, err := random.NewRandomPassword(ctx, "db-password", &random.RandomPasswordArgs{
			Length:          pulumi.Int(20),
			Special:         pulumi.Bool(true),
			OverrideSpecial: pulumi.String(fmt.Sprintf("%v%v%v", "_", "%", "@")),
		})
		if err != nil {
			return err
		}

		// create a new database
		database, err := rds.NewInstance(ctx, "db", &rds.InstanceArgs{
			AllocatedStorage: pulumi.Int(20),
			Engine:           pulumi.String("mysql"),
			EngineVersion:    pulumi.String("5.7"),
			InstanceClass:    pulumi.String("db.t2.micro"),
			Name:             pulumi.String("appdb"),
			StorageType:      pulumi.String("gp2"),
			Password:         dbPassword.Result,
			Username:         pulumi.String("admin"),
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		})

		ctx.Export("arn", database.Arn)
		ctx.Export("endpoint", database.Endpoint)
		ctx.Export("username", database.Username)
		ctx.Export("password", pulumi.ToSecret(database.Password)) // make sure the password is exported as a secret

		return nil
	})
}
