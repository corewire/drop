package pacing

import (
	"context"
	"time"

	v1alpha1 "github.com/corewire/drop/api/v1alpha1"
	"github.com/corewire/drop/internal/podbuilder"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Decision represents whether a new pull is allowed.
type Decision struct {
	Allowed   bool
	RequeueIn time.Duration
}

// Engine evaluates pacing constraints before creating new drop Pods.
type Engine struct {
	Client       client.Client
	PodNamespace string
}

// NewEngine creates a new pacing engine.
func NewEngine(c client.Client, podNamespace string) *Engine {
	return &Engine{Client: c, PodNamespace: podNamespace}
}

// CanStartPull checks pacing constraints and returns whether a new pull can start.
func (e *Engine) CanStartPull(ctx context.Context, policy *v1alpha1.PullPolicy, cachedImageName string) (Decision, error) {
	maxConcurrent := int32(1)
	minDelay := 10 * time.Second

	if policy != nil {
		if policy.Spec.MaxConcurrentNodes > 0 {
			maxConcurrent = policy.Spec.MaxConcurrentNodes
		}
		if policy.Spec.MinDelayBetweenPulls.Duration > 0 {
			minDelay = policy.Spec.MinDelayBetweenPulls.Duration
		}
	}

	// List active drop Pods (Running or Pending)
	podList := &corev1.PodList{}
	ns := e.PodNamespace
	if ns == "" {
		ns = podbuilder.DefaultPodNamespace
	}
	listOpts := []client.ListOption{
		client.InNamespace(ns),
		client.MatchingLabels{podbuilder.LabelManagedBy: podbuilder.LabelManagedByValue},
	}
	if err := e.Client.List(ctx, podList, listOpts...); err != nil {
		return Decision{}, err
	}

	// Filter to active pods (Pending or Running) and optionally scope by node selector
	var activePods []corev1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase == corev1.PodPending || pod.Status.Phase == corev1.PodRunning {
			// Skip pods stuck in image pull errors — they're about to be cleaned up
			if isStuckImagePull(pod) {
				continue
			}
			if policy != nil && len(policy.Spec.NodeSelector) > 0 {
				if !nodeMatchesSelector(pod.Spec.NodeName, policy.Spec.NodeSelector) {
					continue
				}
			}
			activePods = append(activePods, *pod)
		}
	}

	// Check concurrent limit
	if int32(len(activePods)) >= maxConcurrent {
		return Decision{Allowed: false, RequeueIn: 5 * time.Second}, nil
	}

	// Check minimum delay between pulls
	var mostRecent time.Time
	for i := range activePods {
		created := activePods[i].CreationTimestamp.Time
		if created.After(mostRecent) {
			mostRecent = created
		}
	}

	if !mostRecent.IsZero() {
		elapsed := time.Since(mostRecent)
		if elapsed < minDelay {
			remaining := minDelay - elapsed
			return Decision{Allowed: false, RequeueIn: remaining}, nil
		}
	}

	return Decision{Allowed: true}, nil
}

// nodeMatchesSelector is a simplified check.
// In a real implementation, we'd look up the node's labels.
// For now, this always returns true since drop Pods are already placed
// on specific nodes via nodeName — the pacing scope is informational.
func nodeMatchesSelector(_ string, _ map[string]string) bool {
	return true
}

// isStuckImagePull returns true if a pod has a container waiting due to image pull failure.
func isStuckImagePull(pod *corev1.Pod) bool {
	for _, cs := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
		if cs.State.Waiting != nil {
			switch cs.State.Waiting.Reason {
			case "ErrImagePull", "ImagePullBackOff", "InvalidImageName", "RegistryUnavailable":
				return true
			}
		}
	}
	return false
}
