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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dropv1alpha1 "github.com/corewire/drop/api/v1alpha1"
	"github.com/corewire/drop/internal/pacing"
)

var _ = Describe("CachedImage Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-cachedimage"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		cachedimage := &dropv1alpha1.CachedImage{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind CachedImage")
			err := k8sClient.Get(ctx, typeNamespacedName, cachedimage)
			if err != nil && errors.IsNotFound(err) {
				resource := &dropv1alpha1.CachedImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: dropv1alpha1.CachedImageSpec{
						Image: "docker.io/library/nginx",
						Tag:   "1.25",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &dropv1alpha1.CachedImage{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance CachedImage")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &CachedImageReconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				PodNamespace: "drop-system",
				PacingEngine: pacing.NewEngine(k8sClient, "drop-system"),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
