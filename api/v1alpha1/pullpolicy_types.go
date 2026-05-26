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

// PullPolicySpec defines pacing and behavior configuration for image pulls.
type PullPolicySpec struct {
	// MaxConcurrentNodes is the max nodes pulling simultaneously for this policy.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	MaxConcurrentNodes int32 `json:"maxConcurrentNodes,omitempty"`
	// MinDelayBetweenPulls is the minimum time between starting pulls on different nodes.
	// +kubebuilder:default="10s"
	MinDelayBetweenPulls metav1.Duration `json:"minDelayBetweenPulls,omitempty"`
	// FailureBackoff configures retry delays on pull failures.
	// +optional
	FailureBackoff *BackoffConfig `json:"failureBackoff,omitempty"`
	// RepullInterval is how often to re-pull cached images. Zero or unset means never re-pull.
	// +optional
	RepullInterval *metav1.Duration `json:"repullInterval,omitempty"`
	// NodeSelector scopes this policy to a specific node pool.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations match tainted nodes in the pool.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// BackoffConfig defines retry backoff behavior.
type BackoffConfig struct {
	// Initial delay before first retry.
	// +kubebuilder:default="30s"
	Initial metav1.Duration `json:"initial,omitempty"`
	// Max delay cap for exponential backoff.
	// +kubebuilder:default="5m"
	Max metav1.Duration `json:"max,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=drop
// +kubebuilder:printcolumn:name="MaxNodes",type=integer,JSONPath=`.spec.maxConcurrentNodes`
// +kubebuilder:printcolumn:name="MinDelay",type=string,JSONPath=`.spec.minDelayBetweenPulls`
// +kubebuilder:printcolumn:name="RepullInterval",type=string,JSONPath=`.spec.repullInterval`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PullPolicy is the Schema for the pullpolicies API.
// It is a configuration-only resource with no status.
type PullPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PullPolicySpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// PullPolicyList contains a list of PullPolicy.
type PullPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PullPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &PullPolicy{}, &PullPolicyList{})
		return nil
	})
}
