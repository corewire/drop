package podbuilder

import (
	"testing"

	v1alpha1 "github.com/Breee/puller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildPullerPod(t *testing.T) {
	tests := []struct {
		name     string
		ci       *v1alpha1.CachedImage
		nodeName string
		wantImg  string
		wantPull corev1.PullPolicy
	}{
		{
			name: "image with tag",
			ci: &v1alpha1.CachedImage{
				ObjectMeta: metav1.ObjectMeta{Name: "test-image", UID: "uid-1"},
				Spec: v1alpha1.CachedImageSpec{
					Image:      "docker.io/library/nginx",
					Tag:        "1.25",
					PullPolicy: "IfNotPresent",
				},
			},
			nodeName: "node-1",
			wantImg:  "docker.io/library/nginx:1.25",
			wantPull: corev1.PullIfNotPresent,
		},
		{
			name: "image with digest",
			ci: &v1alpha1.CachedImage{
				ObjectMeta: metav1.ObjectMeta{Name: "digest-image", UID: "uid-2"},
				Spec: v1alpha1.CachedImageSpec{
					Image:      "docker.io/library/nginx",
					Digest:     "sha256:abc123",
					PullPolicy: "IfNotPresent",
				},
			},
			nodeName: "node-2",
			wantImg:  "docker.io/library/nginx@sha256:abc123",
			wantPull: corev1.PullIfNotPresent,
		},
		{
			name: "image with Always pull policy",
			ci: &v1alpha1.CachedImage{
				ObjectMeta: metav1.ObjectMeta{Name: "always-pull", UID: "uid-3"},
				Spec: v1alpha1.CachedImageSpec{
					Image:      "gcr.io/my-project/app",
					Tag:        "latest",
					PullPolicy: "Always",
				},
			},
			nodeName: "node-3",
			wantImg:  "gcr.io/my-project/app:latest",
			wantPull: corev1.PullAlways,
		},
		{
			name: "image with no tag defaults to latest",
			ci: &v1alpha1.CachedImage{
				ObjectMeta: metav1.ObjectMeta{Name: "no-tag", UID: "uid-4"},
				Spec: v1alpha1.CachedImageSpec{
					Image: "docker.io/library/alpine",
				},
			},
			nodeName: "node-1",
			wantImg:  "docker.io/library/alpine:latest",
			wantPull: corev1.PullIfNotPresent,
		},
		{
			name: "image with tolerations",
			ci: &v1alpha1.CachedImage{
				ObjectMeta: metav1.ObjectMeta{Name: "tolerated", UID: "uid-5"},
				Spec: v1alpha1.CachedImageSpec{
					Image: "docker.io/library/alpine",
					Tag:   "3.18",
					Tolerations: []corev1.Toleration{
						{Key: "node-role.kubernetes.io/build", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
					},
				},
			},
			nodeName: "build-node-1",
			wantImg:  "docker.io/library/alpine:3.18",
			wantPull: corev1.PullIfNotPresent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod, err := BuildPullerPod(tt.ci, tt.nodeName, "puller-system")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check namespace
			if pod.Namespace != "puller-system" {
				t.Errorf("namespace = %q, want %q", pod.Namespace, "puller-system")
			}

			// Check nodeName
			if pod.Spec.NodeName != tt.nodeName {
				t.Errorf("nodeName = %q, want %q", pod.Spec.NodeName, tt.nodeName)
			}

			// Check image reference
			if pod.Spec.Containers[0].Image != tt.wantImg {
				t.Errorf("image = %q, want %q", pod.Spec.Containers[0].Image, tt.wantImg)
			}

			// Check pull policy
			if pod.Spec.Containers[0].ImagePullPolicy != tt.wantPull {
				t.Errorf("imagePullPolicy = %q, want %q", pod.Spec.Containers[0].ImagePullPolicy, tt.wantPull)
			}

			// Check labels
			if pod.Labels[LabelManagedBy] != LabelManagedByValue {
				t.Errorf("managed-by label = %q, want %q", pod.Labels[LabelManagedBy], LabelManagedByValue)
			}
			if pod.Labels[LabelCachedImage] != tt.ci.Name {
				t.Errorf("cachedimage label = %q, want %q", pod.Labels[LabelCachedImage], tt.ci.Name)
			}
			if pod.Labels[LabelNode] != tt.nodeName {
				t.Errorf("node label = %q, want %q", pod.Labels[LabelNode], tt.nodeName)
			}

			// Check command
			if len(pod.Spec.Containers[0].Command) != 1 || pod.Spec.Containers[0].Command[0] != "true" {
				t.Errorf("command = %v, want [true]", pod.Spec.Containers[0].Command)
			}

			// Check restart policy
			if pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
				t.Errorf("restartPolicy = %q, want Never", pod.Spec.RestartPolicy)
			}

			// Check tolerations
			if len(tt.ci.Spec.Tolerations) > 0 {
				if len(pod.Spec.Tolerations) != len(tt.ci.Spec.Tolerations) {
					t.Errorf("tolerations count = %d, want %d", len(pod.Spec.Tolerations), len(tt.ci.Spec.Tolerations))
				}
			}

			// Check security settings
			if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
				t.Error("automountServiceAccountToken should be false")
			}
			if pod.Spec.EnableServiceLinks == nil || *pod.Spec.EnableServiceLinks {
				t.Error("enableServiceLinks should be false")
			}
			if pod.Spec.TerminationGracePeriodSeconds == nil || *pod.Spec.TerminationGracePeriodSeconds != 0 {
				t.Error("terminationGracePeriodSeconds should be 0")
			}
		})
	}
}
