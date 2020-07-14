package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ec2"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/rds"
	"github.com/pulumi/pulumi-random/sdk/v2/go/random"
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
		 * we have to do some type casting here to ensure the subnets are in the right format
		 * we need for the subnet group
		 */
		subnets := pulumi.StringArrayOutput(vpc.GetOutput(pulumi.String("privateSubnets")))

		/*
		 * Create an RDS subnet group which can be used by the database
		 */
		dbSubnetGroup, err := rds.NewSubnetGroup(ctx, "db-subnet-group", &rds.SubnetGroupArgs{
			SubnetIds: subnets,
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		})
		if err != nil {
			return err
		}

		/*
		 * Create a security group to authorize access to the database
		 */
		dbSecurityGroup, err := ec2.NewSecurityGroup(ctx, "rds-db-security-group", &ec2.SecurityGroupArgs{
			Description: pulumi.String("Allow traffic into RDS database"),
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
					FromPort: pulumi.Int(3306),
					ToPort:   pulumi.Int(3306),
					CidrBlocks: pulumi.StringArray{
						pulumi.String("172.1.0.0/16"),
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
		/*
		 * Generate a random password using the random provider
		 */
		dbPassword, err := random.NewRandomPassword(ctx, "db-password", &random.RandomPasswordArgs{
			Length: pulumi.Int(20),
		})
		if err != nil {
			return err
		}

		/*
		 * Create a new database
		 */
		database, err := rds.NewInstance(ctx, "db", &rds.InstanceArgs{
			AllocatedStorage:        pulumi.Int(20),
			Engine:                  pulumi.String("mysql"),
			EngineVersion:           pulumi.String("5.7"),
			InstanceClass:           pulumi.String("db.t2.micro"),
			FinalSnapshotIdentifier: pulumi.String("lbriggs-db"),
			Name:                    pulumi.String("appdb"),
			StorageType:             pulumi.String("gp2"),
			Password:                dbPassword.Result,
			Username:                pulumi.String("admin"),
			DbSubnetGroupName:       dbSubnetGroup.Name,
			VpcSecurityGroupIds: pulumi.StringArray{
				dbSecurityGroup.ID(),
			},
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
