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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CachedImageSetSpec defines the desired state of CachedImageSet.
type CachedImageSetSpec struct {
	// PolicyRef references a PullPolicy for pacing controls.
	// +optional
	PolicyRef *PolicyReference `json:"policyRef,omitempty"`
	// DiscoveryPolicyRef references a DiscoveryPolicy for dynamic image lists.
	// +optional
	DiscoveryPolicyRef *DiscoveryPolicyReference `json:"discoveryPolicyRef,omitempty"`
	// ImagePullPolicy controls when kubelet pulls the image (propagated to children).
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +kubebuilder:default=Always
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// ImagePullSecrets are references to secrets for pulling from private registries (propagated to children).
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// NodeSelector restricts which nodes to cache images on (propagated to children).
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations allow targeting tainted nodes (propagated to children).
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// Images is a static list of images to cache.
	// +optional
	Images []ImageEntry `json:"images,omitempty"`
}

// ImageEntry defines a single image to include in a set.
type ImageEntry struct {
	// Image is the fully qualified image reference (registry/repository).
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Tag to pull.
	// +optional
	Tag string `json:"tag,omitempty"`
	// Digest to pull.
	// +optional
	Digest string `json:"digest,omitempty"`
}

// DiscoveryPolicyReference is a reference to a DiscoveryPolicy resource.
type DiscoveryPolicyReference struct {
	// Name of the DiscoveryPolicy resource.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// CachedImageSetStatus defines the observed state of CachedImageSet.
type CachedImageSetStatus struct {
	// ObservedGeneration is the last generation reconciled.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Phase summarizes the overall state.
	// +kubebuilder:validation:Enum=Pending;Ready;Degraded
	Phase string `json:"phase,omitempty"`
	// ImagesManaged is the number of CachedImage children managed by this set.
	ImagesManaged int32 `json:"imagesManaged,omitempty"`
	// ImagesReady is the number of children in Ready phase.
	ImagesReady int32 `json:"imagesReady,omitempty"`
	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories=puller
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.imagesReady`
// +kubebuilder:printcolumn:name="Managed",type=integer,JSONPath=`.status.imagesManaged`
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.discoveryPolicyRef.name`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CachedImageSet is the Schema for the cachedimagesets API.
type CachedImageSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CachedImageSetSpec   `json:"spec,omitempty"`
	Status CachedImageSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CachedImageSetList contains a list of CachedImageSet.
type CachedImageSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CachedImageSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CachedImageSet{}, &CachedImageSetList{})
}
