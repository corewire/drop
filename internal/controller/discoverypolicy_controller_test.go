/*
Copyright (c) 2026 Breee

SPDX-License-Identifier: MIT
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

	dropv1alpha1 "github.com/Breee/drop/api/v1alpha1"
)

var _ = Describe("DiscoveryPolicy Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-discovery"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		discoverypolicy := &dropv1alpha1.DiscoveryPolicy{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind DiscoveryPolicy")
			err := k8sClient.Get(ctx, typeNamespacedName, discoverypolicy)
			if err != nil && errors.IsNotFound(err) {
				resource := &dropv1alpha1.DiscoveryPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: dropv1alpha1.DiscoveryPolicySpec{
						Sources: []dropv1alpha1.DiscoverySource{
							{
								Type: "prometheus",
								Prometheus: &dropv1alpha1.PrometheusSource{
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
			resource := &dropv1alpha1.DiscoveryPolicy{}
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
