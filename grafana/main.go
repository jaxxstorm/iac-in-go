package main

import (
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi-random/sdk/v2/go/random"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/lb"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/route53"
	"github.com/pulumi/pulumi-mysql/sdk/v2/go/mysql"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		/*
		 * Grab the ecs cluster stack outputs
		 */
		ecsSlug := fmt.Sprintf("jaxxstorm/ecs.go/%v", ctx.Stack())
		cluster, err := pulumi.NewStackReference(ctx, ecsSlug, nil)
		if err != nil {
			return fmt.Errorf("Error getting ecs stack reference: %w", err)
		}

		/*
		 * Grab the vpc cluster stack outputs
		 */
		vpcSlug := fmt.Sprintf("jaxxstorm/vpc.go/%v", ctx.Stack())
		vpc, err := pulumi.NewStackReference(ctx, vpcSlug, nil)
		if err != nil {
			return fmt.Errorf("Error getting vpc stack reference: %w", err)
		}

		/*
		 * Grab the load balancer stack outputs
		 */
		albSlug := fmt.Sprintf("jaxxstorm/alb.go/%v", ctx.Stack())
		alb, err := pulumi.NewStackReference(ctx, albSlug, nil)
		if err != nil {
			return fmt.Errorf("Error getting alb stack reference: %w", err)
		}

		/*
		 * Grab the load balancer stack outputs
		 */
		dbSlug := fmt.Sprintf("jaxxstorm/db.go/%v", ctx.Stack())
		db, err := pulumi.NewStackReference(ctx, dbSlug, nil)
		if err != nil {
			return fmt.Errorf("Error getting db stack reference: %w", err)
		}

		/*
		 * Create a security group for the grafana task
		 * it needs to allow access on the container port
		 * FIXME: only allow the ALB security
		 */
		grafanaSecurityGroup, err := ec2.NewSecurityGroup(ctx, "grafana", &ec2.SecurityGroupArgs{
			VpcId:       vpc.GetStringOutput(pulumi.String("id")),
			Description: pulumi.String("Web security for ALB"),
			Ingress: &ec2.SecurityGroupIngressArray{
				&ec2.SecurityGroupIngressArgs{
					Protocol: pulumi.String("tcp"),
					FromPort: pulumi.Int(3000),
					ToPort:   pulumi.Int(3000),
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

		/*
		 * Create a targetgroup for grafana which targets the
		 * Fargate. It takes the VPC Id as an input
		 * because that's how fargate networking works
		 */
		grafanaTargetGroup, err := lb.NewTargetGroup(ctx, "grafana", &lb.TargetGroupArgs{
			Port:       pulumi.Int(3000),
			Protocol:   pulumi.String("HTTP"),
			TargetType: pulumi.String("ip"),
			VpcId:      vpc.GetStringOutput(pulumi.String("id")),
			HealthCheck: &lb.TargetGroupHealthCheckArgs{
				Path: pulumi.String("/api/health"),
			},
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		})
		if err != nil {
			return err
		}

		/*
		 * Create a grafana listener rule
		   It forwards all traffic to the grafana listener rule
		*/
		_, err = lb.NewListenerRule(ctx, "grafana", &lb.ListenerRuleArgs{
			Actions: &lb.ListenerRuleActionArray{
				&lb.ListenerRuleActionArgs{
					TargetGroupArn: grafanaTargetGroup.Arn,
					Type:           pulumi.String("forward"),
				},
			},
			Conditions: &lb.ListenerRuleConditionArray{
				&lb.ListenerRuleConditionArgs{
					HostHeader: &lb.ListenerRuleConditionHostHeaderArgs{
						Values: pulumi.StringArray{
							pulumi.String("grafana.aws.briggs.work"), // FIXME: make the host header configurable
						},
					},
				},
			},
			ListenerArn: alb.GetStringOutput(pulumi.String("httpsListenerArn")),
			Priority:    pulumi.Int(200),
		}, pulumi.Parent(grafanaTargetGroup))
		if err != nil {
			return err
		}

		/*
		 * Add a route53 record for grafana which points at the ALB
		 */
		grafanaRoute53Record, err := route53.NewRecord(ctx, "grafana", &route53.RecordArgs{
			Name: pulumi.String("grafana.aws.briggs.work"),
			Records: pulumi.StringArray{
				alb.GetStringOutput(pulumi.String("dnsName")),
			},
			Ttl:    pulumi.Int(300),
			Type:   pulumi.String("CNAME"),
			ZoneId: pulumi.String("Z08976112HCEEMBICZ9N0"),
		})
		if err != nil {
			return err
		}

		/*
		 * Set up the MySQL database
		 */
		dbProvider, err := mysql.NewProvider(ctx, "db-provider", &mysql.ProviderArgs{
			// Endpoint: db.GetStringOutput(pulumi.String("endpoint")),
			Endpoint: pulumi.String("db399f5eb.chuqccm8uxqx.us-west-2.rds.amazonaws.com"),
			Username: db.GetStringOutput(pulumi.String("username")),
			Password: db.GetStringOutput(pulumi.String("password")),
		})
		if err != nil {
			return err
		}

		_ = dbProvider

		/*
		 * Generate a random password using the random provider
		 */
		grafanaUserPassword, err := random.NewRandomPassword(ctx, "db-password", &random.RandomPasswordArgs{
			Length: pulumi.Int(20),
		}, pulumi.Provider(dbProvider))
		if err != nil {
			return err
		}

		_ = grafanaUserPassword

		/*
			grafanaDatabase, err := mysql.NewDatabase(ctx, "grafana", &mysql.DatabaseArgs{
				Name: pulumi.String("grafana"),
			}, pulumi.Provider(dbProvider))
			if err != nil {
				return err
			}
		*/

		grafanaUser, err := mysql.NewUser(ctx, "grafana", &mysql.UserArgs{
			User:              pulumi.String("grafana"),
			PlaintextPassword: grafanaUserPassword.Result,
		}, pulumi.Provider(dbProvider))

		_ = grafanaUser

		/*
			_, err = mysql.NewGrant(ctx, "grafana", &mysql.GrantArgs{
				User:     grafanaUser.User,
				Database: grafanaDatabase.Name,
				Privileges: pulumi.StringArray{
					pulumi.String("ALL"),
				},
			}, pulumi.Provider(dbProvider))
		*/

		/*
		 * Define the JSON task definition for grafana
		 */
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
			/*
				"environment": []interface{}{
					map[string]interface{}{
						"name":  "GF_DATABASE_HOST",
						"value": 3000,
					},
					map[string]interface{}{
						"name":  "GF_DATABASE_TYPE",
						"value": 3000,
					},
					map[string]interface{}{
						"name":  "GF_DATABASE_USER",
						"value": "grafana",
					},
					map[string]interface{}{
						"name":  "GF_DATABASE_PASSWORD",
						"value": "grafana",
					},
					map[string]interface{}{
						"name":  "GF_AUTH_ANONYMOUS_ENABLED",
						"value": "true",
					},
					map[string]interface{}{
						"name":  "GF_SECURITY_ADMIN_PASSWORD",
						"value": "admin",
					},
					map[string]interface{}{
						"name":  "GF_SECURITY_ROOT_URL",
						"value": "https://grafana.aws.briggs.work",
					},
				},
			*/
		}})
		if err != nil {
			return err
		}

		/*
		 * Define an ECS task definition
		 * we use the task definition from above as an input
		 */
		grafanaTaskDefinition, err := ecs.NewTaskDefinition(ctx, "grafana", &ecs.TaskDefinitionArgs{
			Family:                  pulumi.String("grafana"),
			Cpu:                     pulumi.String("256"),
			Memory:                  pulumi.String("512"),
			NetworkMode:             pulumi.String("awsvpc"),
			RequiresCompatibilities: pulumi.StringArray{pulumi.String("FARGATE")},
			ExecutionRoleArn:        cluster.GetStringOutput(pulumi.String("taskExecRoleArn")),
			ContainerDefinitions:    pulumi.String(grafanaTaskDefinitionJSON),
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		})
		if err != nil {
			return err
		}

		/*
		 * Define an ECS service
		 *
		 */
		_, err = ecs.NewService(ctx, "grafana", &ecs.ServiceArgs{
			Cluster:        cluster.GetStringOutput(pulumi.String("clusterArn")),
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
				SecurityGroups: pulumi.StringArray{
					grafanaSecurityGroup.ID().ToStringOutput(),
				},
			},
			LoadBalancers: &ecs.ServiceLoadBalancerArray{
				&ecs.ServiceLoadBalancerArgs{
					TargetGroupArn: grafanaTargetGroup.Arn,
					ContainerName:  pulumi.String("grafana"),
					ContainerPort:  pulumi.Int(3000),
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
		 * we only need to output the used address
		 */
		ctx.Export("address", grafanaRoute53Record.Name)

		return nil
	})
}

/*
 * A helper function to convert strings to StringArrays
 */
func toPulumiStringArray(a []string) pulumi.StringArrayInput {
	var res []pulumi.StringInput
	for _, s := range a {
		res = append(res, pulumi.String(s))
	}
	return pulumi.StringArray(res)
}
