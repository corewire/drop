package podbuilder

import (
	"fmt"

	v1alpha1 "github.com/Breee/puller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	// LabelManagedBy identifies resources managed by the puller operator.
	LabelManagedBy = "app.kubernetes.io/managed-by"
	// LabelManagedByValue is the value for the managed-by label.
	LabelManagedByValue = "puller"
	// LabelCachedImage identifies which CachedImage owns this Pod.
	LabelCachedImage = "puller.corewire.io/cachedimage"
	// LabelNode identifies which node this Pod targets.
	LabelNode = "puller.corewire.io/node"
	// DefaultPodNamespace is the namespace where puller pods are created.
	DefaultPodNamespace = "puller-system"
)

// BuildPullerPod creates a Pod spec for pulling an image onto a specific node.
// Pods are created in the given namespace and tracked via labels (not ownerRefs)
// because CachedImage is cluster-scoped and cannot own namespaced resources.
func BuildPullerPod(ci *v1alpha1.CachedImage, nodeName, namespace string) (*corev1.Pod, error) {
	imageRef := buildImageRef(ci)

	pullPolicy := corev1.PullIfNotPresent
	if ci.Spec.PullPolicy == "Always" {
		pullPolicy = corev1.PullAlways
	}

	if namespace == "" {
		namespace = DefaultPodNamespace
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("puller-%s-", ci.Name),
			Namespace:    namespace,
			Labels: map[string]string{
				LabelManagedBy:   LabelManagedByValue,
				LabelCachedImage: ci.Name,
				LabelNode:        nodeName,
			},
		},
		Spec: corev1.PodSpec{
			NodeName:      nodeName,
			RestartPolicy: corev1.RestartPolicyNever,
			Tolerations:   ci.Spec.Tolerations,
			Containers: []corev1.Container{
				{
					Name:            "pull",
					Image:           imageRef,
					Command:         []string{"true"},
					ImagePullPolicy: pullPolicy,
				},
			},
			AutomountServiceAccountToken:  ptr.To(false),
			EnableServiceLinks:            ptr.To(false),
			TerminationGracePeriodSeconds: ptr.To(int64(0)),
		},
	}

	return pod, nil
}

// buildImageRef constructs the full image reference from CachedImage spec.
func buildImageRef(ci *v1alpha1.CachedImage) string {
	if ci.Spec.Digest != "" {
		return fmt.Sprintf("%s@%s", ci.Spec.Image, ci.Spec.Digest)
	}
	tag := ci.Spec.Tag
	if tag == "" {
		tag = "latest"
	}
	return fmt.Sprintf("%s:%s", ci.Spec.Image, tag)
}
