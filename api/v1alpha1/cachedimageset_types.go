/*
Copyright (c) 2026 Breee

SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// CachedImageSetSpec defines the desired state of CachedImageSet.
// A CachedImageSet creates and manages child CachedImage resources automatically.
// All spec fields (nodeSelector, tolerations, imagePullPolicy, imagePullSecrets, policyRef)
// are propagated to every child CachedImage.
type CachedImageSetSpec struct {
	// PolicyRef references a PullPolicy for pacing controls. Propagated to all child CachedImages.
	// Example: {name: "conservative"}
	// +optional
	PolicyRef *PolicyReference `json:"policyRef,omitempty"`
	// DiscoveryPolicyRef references a DiscoveryPolicy that provides a dynamic image list.
	// When set, the operator reads status.discoveredImages from the referenced DiscoveryPolicy
	// and creates/deletes child CachedImages accordingly. Can be combined with static images.
	// Example: {name: "popular-build-images"}
	// +optional
	DiscoveryPolicyRef *DiscoveryPolicyReference `json:"discoveryPolicyRef,omitempty"`
	// ImagePullPolicy controls when kubelet pulls images. Propagated to all child CachedImages.
	// Default: "Always". See CachedImage.spec.imagePullPolicy for details.
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +kubebuilder:default=Always
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// ImagePullSecrets for private registries. Propagated to all child CachedImages.
	// Example: [{name: "ghcr-creds"}]
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// NodeSelector restricts which nodes to cache images on. Propagated to all child CachedImages.
	// Example: {"node-role.kubernetes.io/build": "true"}
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations for tainted nodes. Propagated to all child CachedImages.
	// Example: [{key: "node-role.kubernetes.io/build", operator: "Exists", effect: "NoSchedule"}]
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// Images is a static list of images to cache. Each entry creates one child CachedImage.
	// Can be used alone or combined with discoveryPolicyRef (both lists are merged).
	// +optional
	Images []ImageEntry `json:"images,omitempty"`
}

// ImageEntry defines a single image to include in a set.
type ImageEntry struct {
	// Image is the fully qualified image reference without tag or digest.
	// Example: "docker.io/library/nginx", "registry.example.com/team/app"
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Tag to pull. Mutually exclusive with Digest.
	// Example: "1.25-alpine", "v2.4.1"
	// +optional
	Tag string `json:"tag,omitempty"`
	// Digest to pull as an immutable reference. Mutually exclusive with Tag.
	// Example: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4"
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
// +kubebuilder:resource:scope=Cluster,categories=drop
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.imagesReady`
// +kubebuilder:printcolumn:name="Managed",type=integer,JSONPath=`.status.imagesManaged`
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.discoveryPolicyRef.name`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CachedImageSet manages a group of images to cache, optionally backed by a DiscoveryPolicy.
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
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &CachedImageSet{}, &CachedImageSetList{})
		return nil
	})
}
