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

// CachedImageSpec defines the desired state of CachedImage.
type CachedImageSpec struct {
	// Image is the fully qualified image reference without tag or digest.
	// Example: "docker.io/library/nginx", "registry.example.com/team/app"
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Tag to pull. Mutually exclusive with Digest.
	// Example: "1.25-alpine", "v2.4.1", "latest"
	// +optional
	Tag string `json:"tag,omitempty"`
	// Digest to pull as an immutable reference. Mutually exclusive with Tag.
	// Use this for reproducible deployments where the exact image layer matters.
	// Example: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4"
	// +optional
	Digest string `json:"digest,omitempty"`
	// ImagePullPolicy controls when kubelet pulls the image on each node.
	// - Always (default): check the registry for a newer digest even if the tag exists locally.
	// - IfNotPresent: skip the registry check when the tag already exists on the node.
	// - Never: never pull (only useful for pre-loaded images).
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +kubebuilder:default=Always
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// ImagePullSecrets are references to Secrets in the namespace where Drop creates pull Pods.
	// The default namespace is "drop-system" unless the controller is started with a different --pod-namespace.
	// The Secret must contain a .dockerconfigjson key.
	// Example: [{name: "ghcr-creds"}, {name: "ecr-creds"}]
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// NodeSelector restricts which nodes to cache the image on.
	// Only nodes matching ALL key-value pairs will be targeted.
	// Example: {"node-role.kubernetes.io/build": "true"}
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations allow the pull pod to be scheduled on tainted nodes.
	// Example: [{key: "node-role.kubernetes.io/build", operator: "Exists", effect: "NoSchedule"}]
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// Priority is a pull ordering hint. Lower values are pulled first.
	// Images with the same priority are pulled in alphabetical order.
	// Default: 0 (no priority). Example: 10 (low priority), -10 (high priority)
	// +optional
	Priority *int32 `json:"priority,omitempty"`
	// PolicyRef references a PullPolicy resource that controls pacing (concurrency, backoff, delays).
	// If unset, the operator uses built-in defaults (1 concurrent node, 10s delay, 30s initial backoff).
	// Example: {name: "conservative"}
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

// CachedImage ensures a single container image is pre-cached on cluster nodes.
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
