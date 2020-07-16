package main

import (
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes"

	"github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/apiextensions"

	"github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/yaml"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v2/go/kubernetes/core/v1"
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

		namespace, err := corev1.NewNamespace(ctx, "sock-shop", &corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String("sock-shop"),
			},
		}, pulumi.Provider(provider))
		if err != nil {
			return err
		}

		_, err = yaml.NewConfigFile(ctx, "sock-shop", &yaml.ConfigFileArgs{
			File: "manifests/complete-demo.yaml",
			Transformations: []yaml.Transformation{
				func(state map[string]interface{}, opts ...pulumi.ResourceOption) {
					if state["apiVersion"] == "extensions/v1beta1" {
						state["apiVersion"] = "apps/v1"
					}
				},
			},
		}, pulumi.Provider(provider), pulumi.Parent(namespace))
		if err != nil {
			return err
		}

		sockShopIngress, err := networkingv1beta1.NewIngress(ctx, "sock-shop-ingress", &networkingv1beta1.IngressArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("web"),
				Namespace: pulumi.String("sock-shop"),
				Annotations: pulumi.StringMap{
					"konghq.com/override":   pulumi.String("https-redirect"),
					"konghq.com/strip-path": pulumi.String("true"),
				},
			},
			Spec: &networkingv1beta1.IngressSpecArgs{
				Rules: &networkingv1beta1.IngressRuleArray{
					networkingv1beta1.IngressRuleArgs{
						Host: pulumi.String("sock-shop.aws.briggs.work"),
						Http: &networkingv1beta1.HTTPIngressRuleValueArgs{
							Paths: networkingv1beta1.HTTPIngressPathArray{
								networkingv1beta1.HTTPIngressPathArgs{
									Path: pulumi.String("/"),
									Backend: networkingv1beta1.IngressBackendArgs{
										ServiceName: pulumi.String("front-end"),
										ServicePort: pulumi.Int(80),
									},
								},
							},
						},
					},
				},
			},
		}, pulumi.Provider(provider), pulumi.Parent(namespace))

		_, err = apiextensions.NewCustomResource(ctx, "sock-shop-kong-ingress", &apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("configuration.konghq.com/v1"),
			Kind:       pulumi.String("KongIngress"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("https-redirect"),
				Namespace: pulumi.String("sock-shop"),
			},
			OtherFields: kubernetes.UntypedArgs{
				"route": kubernetes.UntypedArgs{
					"protocols": []string{"http", "https"},
					"hosts":     []string{"sock-shop.aws.briggs.work"},
				},
			},
		}, pulumi.Provider(provider), pulumi.Parent(sockShopIngress))

		if err != nil {
			return fmt.Errorf("error creating chart: %w", err)
		}

        ctx.Export("address",  pulumi.String("sock-shop.aws.briggs.work"))

		return nil
	})
}
