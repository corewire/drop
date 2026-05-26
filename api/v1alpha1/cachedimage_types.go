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
	"k8s.io/apimachinery/pkg/runtime"
)

// CachedImageSpec defines the desired state of CachedImage.
type CachedImageSpec struct {
	// Image is the fully qualified image reference (registry/repository).
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Tag to pull. Mutually exclusive with Digest.
	// +optional
	Tag string `json:"tag,omitempty"`
	// Digest to pull (immutable reference). Mutually exclusive with Tag.
	// +optional
	Digest string `json:"digest,omitempty"`
	// ImagePullPolicy controls when kubelet pulls the image.
	// Defaults to Always (checks upstream digest, only downloads if changed).
	// Set to IfNotPresent to skip the registry check when the tag already exists locally.
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +kubebuilder:default=Always
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// ImagePullSecrets are references to secrets for pulling from private registries.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// NodeSelector restricts which nodes to cache the image on.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations allow targeting tainted nodes.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// Priority is a pull ordering hint (lower values pulled first).
	// +optional
	Priority *int32 `json:"priority,omitempty"`
	// PolicyRef references a PullPolicy for pacing controls.
	// +optional
	PolicyRef *PolicyReference `json:"policyRef,omitempty"`
}

// PolicyReference is a reference to a PullPolicy resource.
type PolicyReference struct {
	// Name of the PullPolicy resource.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// CachedImageStatus defines the observed state of CachedImage.
type CachedImageStatus struct {
	// ObservedGeneration is the last generation reconciled.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Phase summarizes the overall state.
	// +kubebuilder:validation:Enum=Pending;Pulling;Ready;Degraded
	Phase string `json:"phase,omitempty"`
	// Ready is a human-readable "nodesReady/nodesTargeted" fraction for display.
	Ready string `json:"ready,omitempty"`
	// ResolvedDigest is the sha256 digest of the image as reported by the container runtime after pull.
	// +optional
	ResolvedDigest string `json:"resolvedDigest,omitempty"`
	// NodesTargeted is the number of nodes that should have this image.
	NodesTargeted int32 `json:"nodesTargeted,omitempty"`
	// NodesReady is the number of nodes that have successfully pulled the image.
	NodesReady int32 `json:"nodesReady,omitempty"`
	// NodesPulling is the number of nodes currently pulling the image.
	// +optional
	NodesPulling int32 `json:"nodesPulling,omitempty"`
	// CachedNodes is the list of node names that have successfully cached the image.
	// +optional
	CachedNodes []string `json:"cachedNodes,omitempty"`
	// ConsecutiveFailures counts sequential reconcile failures for backoff calculation.
	// +optional
	ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`
	// LastPulledAt is the timestamp of the most recent successful pull.
	// +optional
	LastPulledAt *metav1.Time `json:"lastPulledAt,omitempty"`
	// LastAttemptedAt is the timestamp of the most recent pull attempt (success or failure).
	// +optional
	LastAttemptedAt *metav1.Time `json:"lastAttemptedAt,omitempty"`
	// Conditions represent the latest available observations.
	// Condition types: Ready, PullProgress.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories=drop
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Tag",type=string,JSONPath=`.spec.tag`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Digest",type=string,JSONPath=`.status.resolvedDigest`,priority=1
// +kubebuilder:printcolumn:name="Set",type=string,JSONPath=`.metadata.labels.drop\.corewire\.io/imageset`,description="Parent CachedImageSet",priority=1
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,priority=1
// +kubebuilder:printcolumn:name="Policy",type=string,JSONPath=`.spec.policyRef.name`,priority=1

// CachedImage is the Schema for the cachedimages API.
type CachedImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CachedImageSpec   `json:"spec,omitempty"`
	Status CachedImageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CachedImageList contains a list of CachedImage.
type CachedImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CachedImage `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &CachedImage{}, &CachedImageList{})
		return nil
	})
}
