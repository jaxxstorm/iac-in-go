package main

import (
	"encoding/json"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		/*
		 * Create an ECS cluster that can run fargate tasks
		 */
		cluster, err := ecs.NewCluster(ctx, "lbriggs-cluster", &ecs.ClusterArgs{
			CapacityProviders: pulumi.StringArray{
				pulumi.String("FARGATE_SPOT"),
				pulumi.String("FARGATE"),
			},
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		})
		if err != nil {
			return err
		}

		/*
		 * IAM policy principal
		 */
		assumeRolePolicyJSON, _ := json.Marshal(map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []interface{}{
				map[string]interface{}{
					"Action": "sts:AssumeRole",
					"Principal": map[string]interface{}{
						"Service": "ecs-tasks.amazonaws.com",
					},
					"Effect": "Allow",
				},
			},
		})

		/*
		 * Create the IAM role that allows the running cluster services to use ECS
		 */
		taskRole, err := iam.NewRole(ctx, "task-exec-role", &iam.RoleArgs{
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
		_, err = iam.NewRolePolicyAttachment(ctx, "task-exec-policy", &iam.RolePolicyAttachmentArgs{
			Role:      taskRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
		}, pulumi.Parent(taskRole))
		if err != nil {
			return err
		}

		ctx.Export("clusterID", cluster.ID())
		ctx.Export("clusterArn", cluster.Arn)
		ctx.Export("taskExecRoleArn", taskRole.Arn)

		return nil
	})
}
