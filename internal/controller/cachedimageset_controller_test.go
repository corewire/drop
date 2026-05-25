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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dropv1alpha1 "github.com/Breee/drop/api/v1alpha1"
)

var _ = Describe("CachedImageSet Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-imageset"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		cachedimageset := &dropv1alpha1.CachedImageSet{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind CachedImageSet")
			err := k8sClient.Get(ctx, typeNamespacedName, cachedimageset)
			if err != nil && errors.IsNotFound(err) {
				resource := &dropv1alpha1.CachedImageSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: dropv1alpha1.CachedImageSetSpec{
						Images: []dropv1alpha1.ImageEntry{
							{Image: "docker.io/library/nginx", Tag: "1.25"},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &dropv1alpha1.CachedImageSet{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance CachedImageSet")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &CachedImageSetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
