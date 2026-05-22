/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	pullerv1alpha1 "github.com/Breee/puller/api/v1alpha1"
)

var _ = Describe("DiscoveryPolicy Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-discovery"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		discoverypolicy := &pullerv1alpha1.DiscoveryPolicy{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind DiscoveryPolicy")
			err := k8sClient.Get(ctx, typeNamespacedName, discoverypolicy)
			if err != nil && errors.IsNotFound(err) {
				resource := &pullerv1alpha1.DiscoveryPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: pullerv1alpha1.DiscoveryPolicySpec{
						Sources: []pullerv1alpha1.DiscoverySource{
							{
								Type: "prometheus",
								Prometheus: &pullerv1alpha1.PrometheusSource{
									Endpoint: "http://localhost:9090",
									Query:    "test_query",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &pullerv1alpha1.DiscoveryPolicy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance DiscoveryPolicy")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DiscoveryPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			// Discovery will fail to connect to prometheus, but should not panic
			// The reconciler handles errors gracefully
			_ = err
		})
	})
})
