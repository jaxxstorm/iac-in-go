package main

import (
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ecs"

	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		// grab the ecs cluster stack outputs
		ecsSlug := fmt.Sprintf("jaxxstorm/ecs.go/%v", ctx.Stack())
		cluster, err := pulumi.NewStackReference(ctx, ecsSlug, nil)
		if err != nil {
			return fmt.Errorf("Error getting ecs stack reference: %w", err)
		}

		// grab the vpc cluster stack outputs
		vpcSlug := fmt.Sprintf("jaxxstorm/vpc.go/%v", ctx.Stack())
		_, err = pulumi.NewStackReference(ctx, vpcSlug, nil)
		if err != nil {
			return fmt.Errorf("Error getting vpc stack reference: %w", err)
		}

		// define the JSON definition for grafana
		grafanaTaskDefinitionJSON, err := json.Marshal([]interface{}{map[string]interface{}{
			"name":  "grafana",
			"image": "grafana/grafana:7.0.3-ubuntu",
			"portMappings": []interface{}{
				map[string]interface{}{
					"containerPort": 3000,
					"hostPort":      3000,
					"protocol":      "tcp",
				},
			},
		}})
		if err != nil {
			return err
		}

		// define an ECS task definition
		grafanaTaskDefinition, err := ecs.NewTaskDefinition(ctx, "grafana", &ecs.TaskDefinitionArgs{
			Family:                  pulumi.String("grafana"),
			Cpu:                     pulumi.String("256"),
			Memory:                  pulumi.String("512"),
			NetworkMode:             pulumi.String("awsvpc"),
			RequiresCompatibilities: pulumi.StringArray{pulumi.String("FARGATE")},
			ExecutionRoleArn:        cluster.GetStringOutput((pulumi.String("taskExecRoleArn"))),
			ContainerDefinitions:    pulumi.String(grafanaTaskDefinitionJSON),
		})
		if err != nil {
			return err
		}

		// define an ECS service
		_, err = ecs.NewService(ctx, "grafana", &ecs.ServiceArgs{
			Cluster:        cluster.GetStringOutput((pulumi.String("clusterArn"))),
			DesiredCount:   pulumi.Int(3),
			LaunchType:     pulumi.String("FARGATE"),
			TaskDefinition: grafanaTaskDefinition.Arn,
			NetworkConfiguration: &ecs.ServiceNetworkConfigurationArgs{
				AssignPublicIp: pulumi.Bool(false),
				// FIXME: use the stack reference here
				Subnets: pulumi.StringArray{
					pulumi.String("subnet-024ab6c39387a96ca"),
					pulumi.String("subnet-03d706c291a35fa13"),
					pulumi.String("subnet-0ecd6c0dc63176ff1"),
				},
			},
		})

		ctx.Export("taskDefinitionArn", grafanaTaskDefinition.Arn)

		return nil
	})
}

func toPulumiStringArray(a []string) pulumi.StringArrayInput {
	var res []pulumi.StringInput
	for _, s := range a {
		res = append(res, pulumi.String(s))
	}
	return pulumi.StringArray(res)
}
