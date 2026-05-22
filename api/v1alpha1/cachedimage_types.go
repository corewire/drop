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
	// PullPolicy controls whether to pull if image exists on node.
	// +kubebuilder:default=IfNotPresent
	// +kubebuilder:validation:Enum=IfNotPresent;Always
	// +optional
	PullPolicy string `json:"pullPolicy,omitempty"`
	// RepullPolicy controls refresh behavior for cached images.
	// +kubebuilder:default=Never
	// +kubebuilder:validation:Enum=Never;OnSchedule;Always
	// +optional
	RepullPolicy string `json:"repullPolicy,omitempty"`
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
	// NodesTargeted is the number of nodes that should have this image.
	NodesTargeted int32 `json:"nodesTargeted,omitempty"`
	// NodesReady is the number of nodes that have successfully pulled the image.
	NodesReady int32 `json:"nodesReady,omitempty"`
	// LastPulledAt is the timestamp of the most recent successful pull.
	// +optional
	LastPulledAt *metav1.Time `json:"lastPulledAt,omitempty"`
	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.nodesReady`
// +kubebuilder:printcolumn:name="Target",type=integer,JSONPath=`.status.nodesTargeted`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

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
	SchemeBuilder.Register(&CachedImage{}, &CachedImageList{})
}
