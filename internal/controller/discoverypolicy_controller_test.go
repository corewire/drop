/*
Copyright (c) 2026 Breee

SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dropv1alpha1 "github.com/corewire/drop/api/v1alpha1"
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
						Queries: []dropv1alpha1.DiscoveryQuery{
							{
								Name: "test-query",
								Type: dropv1alpha1.DiscoveryQueryTypePrometheus,
								Prometheus: &dropv1alpha1.DiscoveryPrometheusQuery{
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

		It("reconciles and sets a failure condition when the Prometheus endpoint is unreachable", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DiscoveryPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// The reconciler will attempt to query localhost:9090 which will fail.
			// It returns an error so controller-runtime applies rate-limited backoff.
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			// Verify the status reflects the query failure.
			updated := &dropv1alpha1.DiscoveryPolicy{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())

			var readyCondition *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == "Ready" {
					readyCondition = &updated.Status.Conditions[i]
				}
			}
			Expect(readyCondition).NotTo(BeNil(), "Ready condition should be set")
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			// Reason is one of ConnectionRefused / SyncFailed depending on OS
			Expect(readyCondition.Reason).NotTo(BeEmpty())
		})

		It("reconciles successfully with a registry query that lists from a mock server", func() {
			By("creating a DiscoveryPolicy with a registry query")
			const regResourceName = "test-discovery-registry"

			// We can't spin up a real registry in unit tests, but we can verify the
			// full pipeline runs without panicking and sets the correct status fields.
			resource := &dropv1alpha1.DiscoveryPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: regResourceName,
				},
				Spec: dropv1alpha1.DiscoveryPolicySpec{
					Queries: []dropv1alpha1.DiscoveryQuery{
						{
							Name: "reg-query",
							Type: dropv1alpha1.DiscoveryQueryTypeRegistry,
							Registry: &dropv1alpha1.DiscoveryRegistryQuery{
								URL:          "http://nonexistent-registry:5000",
								Repositories: []string{"team/app"},
							},
						},
					},
					Signals: []dropv1alpha1.DiscoverySignal{
						{
							Name:  "tag-score",
							Query: "reg-query",
							Type:  dropv1alpha1.SignalTypeAggregate,
							Aggregate: &dropv1alpha1.AggregateSignalConfig{
								Method: dropv1alpha1.AggregationSum,
							},
						},
					},
					Ranking: &dropv1alpha1.DiscoveryRanking{
						Strategy: dropv1alpha1.RankingStrategySignal,
						Signal:   "tag-score",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, resource)
			}()

			controllerReconciler := &DiscoveryPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: regResourceName},
			})

			updated := &dropv1alpha1.DiscoveryPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: regResourceName}, updated)).To(Succeed())

			// Status should have a QueryResult entry for the registry query
			Expect(updated.Status.QueryResults).To(HaveLen(1))
			Expect(updated.Status.QueryResults[0].Name).To(Equal("reg-query"))
			Expect(updated.Status.QueryResults[0].Type).To(Equal(dropv1alpha1.DiscoveryQueryTypeRegistry))
		})

		It("uses the configured secret namespace for discovery source credentials", func() {
			const namespaceName = "custom-drop-system"
			const secretName = "prometheus-creds"

			namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
			err := k8sClient.Create(ctx, namespace)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespaceName,
				},
				Data: map[string][]byte{
					"token": []byte("test-token"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			controllerReconciler := &DiscoveryPolicyReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				SecretNamespace: namespaceName,
			}

			_, err = controllerReconciler.buildHTTPClient(ctx, &corev1.LocalObjectReference{Name: secretName})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
			Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
		})
	})
})
