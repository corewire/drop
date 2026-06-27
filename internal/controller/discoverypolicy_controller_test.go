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

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DiscoveryPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			// The stub reconciler sets a NotImplemented condition and does not return an error.
			Expect(err).NotTo(HaveOccurred())

			// Verify the NotImplemented condition is set in status.
			updated := &dropv1alpha1.DiscoveryPolicy{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
			var readyReason string
			for _, c := range updated.Status.Conditions {
				if c.Type == "Ready" {
					readyReason = c.Reason
				}
			}
			Expect(readyReason).To(Equal("NotImplemented"))
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
