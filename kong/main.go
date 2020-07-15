package main

import (
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/helm/v2"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/meta/v1"
	networkingv1beta1 "github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/networking/v1beta1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/providers"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

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

		namespace, err := corev1.NewNamespace(ctx, "kong", &corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String("kong"),
			},
		}, pulumi.Provider(provider))

		_, err = helm.NewChart(ctx, "kong", helm.ChartArgs{
			Chart: pulumi.String("kong"),
			FetchArgs: &helm.FetchArgs{
				Repo: pulumi.String("https://charts.konghq.com/"),
			},
			Values: pulumi.Map{
				"env": pulumi.Map{
					"database": pulumi.String("off"),
				},
				"ingressController": pulumi.Map{
					"enabled":     pulumi.Bool(true),
					"installCRDs": pulumi.Bool(false),
				},
				"admin": pulumi.Map{
					"enabled": pulumi.Bool(true),
					"http": pulumi.Map{
						"enabled": pulumi.Bool(true),
					},
				},
			},
			Namespace: pulumi.String("kong"),
		}, pulumi.Provider(provider), pulumi.Parent(namespace))

		_, err = helm.NewChart(ctx, "konga", helm.ChartArgs{
			Chart:     pulumi.String("konga"),
			Path:      pulumi.String("./konga"),
			Namespace: pulumi.String("kong"),
		}, pulumi.Provider(provider), pulumi.Parent(namespace))

		_, err = networkingv1beta1.NewIngress(ctx, "konga-ingress", &networkingv1beta1.IngressArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("konga"),
				Namespace: pulumi.String("kong"),
			},
			Spec: &networkingv1beta1.IngressSpecArgs{
				Rules: &networkingv1beta1.IngressRuleArray{
					networkingv1beta1.IngressRuleArgs{
						Host: pulumi.String("konga.aws.briggs.work"),
						Http: &networkingv1beta1.HTTPIngressRuleValueArgs{
							Paths: networkingv1beta1.HTTPIngressPathArray{
								networkingv1beta1.HTTPIngressPathArgs{
									Path: pulumi.String("/"),
									Backend: networkingv1beta1.IngressBackendArgs{
										ServiceName: pulumi.String("konga"),
										ServicePort: pulumi.Int(80),
									},
								},
							},
						},
					},
				},
			},
		}, pulumi.Provider(provider), pulumi.Parent(namespace))

		if err != nil {
			return fmt.Errorf("error creating chart: %w", err)
		}

		return nil
	})
}
