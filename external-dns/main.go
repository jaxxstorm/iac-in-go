package main

import (
	"encoding/json"
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/providers"

	"github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/helm/v2"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		/*
		 * Policy JSON for IAM role
		 */
		externalDNSIAMRolePolicyJSON, err := json.Marshal(map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []interface{}{
				map[string]interface{}{
					"Action": []string{
						"route53:ChangeResourceRecordSets",
					},
					"Effect": "Allow",
					"Resource": []string{
						"arn:aws:route53:::hostedzone/*",
					},
				},
				map[string]interface{}{
					"Action": []string{
						"route53:ListHostedZones",
						"route53:ListResourceRecordSets",
					},
					"Effect":   "Allow",
					"Resource": "*",
				},
			},
		})

		/*
		 * IAM policy principal
		 */
		assumeRolePolicyJSON, _ := json.Marshal(map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []interface{}{
				map[string]interface{}{
					"Effect": "Allow",
					"Principal": map[string]interface{}{
						"Federated": "arn:aws:iam::616138583583:oidc-provider/oidc.eks.us-west-2.amazonaws.com/id/4054561BDB2551CEA4BEF1BA72F66A85",
					},
					"Action": "sts:AssumeRoleWithWebIdentity",
					"Condition": map[string]interface{}{
						"StringEquals": map[string]interface{}{
							"oidc.eks.us-west-2.amazonaws.com/id/4054561BDB2551CEA4BEF1BA72F66A85:sub": "system:serviceaccount:external-dns:external-dns",
						},
					},
				},
			},
		})

		/*
		 * Create the IAM role
		 */
		externalDNSIAMRole, err := iam.NewRole(ctx, "external-dns-iam-role", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(assumeRolePolicyJSON),
			Tags: pulumi.Map{
				"Owner": pulumi.String("lbriggs"),
			},
		})

		/*
		 * Attach a policy
		 */
		route53Policy, err := iam.NewPolicy(ctx, "bastion-ssm-access", &iam.PolicyArgs{
			Policy: pulumi.String(externalDNSIAMRolePolicyJSON),
		}, pulumi.Parent(externalDNSIAMRole))
		if err != nil {
			return err
		}

		_, err = iam.NewRolePolicyAttachment(ctx, "ssm-get-parameters", &iam.RolePolicyAttachmentArgs{
			Role:      externalDNSIAMRole.Name,
			PolicyArn: route53Policy.Arn,
		}, pulumi.Parent(route53Policy))

		/*
		 * Install external dns via helm chart
		 */

		// Get stack reference
		slug := fmt.Sprintf("jaxxstorm/eks.go/%v", ctx.Stack())
		cluster, err := pulumi.NewStackReference(ctx, slug, nil)
		if err != nil {
			return fmt.Errorf("error getting stack reference")
		}

		kubeConfig := cluster.GetOutput(pulumi.String("kubeconfig"))

		// provider init
		provider, err := providers.NewProvider(ctx, "k8sprovider", &providers.ProviderArgs{
			Kubeconfig:                  pulumi.StringPtrOutput(kubeConfig),
			SuppressDeprecationWarnings: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}
		namespace, err := corev1.NewNamespace(ctx, "external-dns", &corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String("external-dns"),
			},
		}, pulumi.Provider(provider))

		_, err = helm.NewChart(ctx, "external-dns", helm.ChartArgs{
			Chart: pulumi.String("external-dns"),
			FetchArgs: &helm.FetchArgs{
				Repo: pulumi.String("https://charts.bitnami.com/bitnami"),
			},
			Values: pulumi.Map{
				"aws": pulumi.Map{
					"region":   pulumi.String("us-west-2"),
					"zoneType": pulumi.String("public"),
				},
				"serviceAccount": pulumi.Map{
					"annotations": pulumi.Map{
						"eks.amazonaws.com/role-arn": externalDNSIAMRole.Arn},
				},
			},
			Namespace: pulumi.String("external-dns"),
		}, pulumi.Provider(provider), pulumi.Parent(namespace))

		return nil
	})
}
