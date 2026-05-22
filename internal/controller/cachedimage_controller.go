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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	pullerv1alpha1 "github.com/Breee/puller/api/v1alpha1"
	"github.com/Breee/puller/internal/pacing"
	"github.com/Breee/puller/internal/podbuilder"
)

const (
	conditionTypeReady = "Ready"
)

// CachedImageReconciler reconciles a CachedImage object
type CachedImageReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	PacingEngine *pacing.Engine
}

// +kubebuilder:rbac:groups=puller.corewire.io,resources=cachedimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=puller.corewire.io,resources=cachedimages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=puller.corewire.io,resources=cachedimages/finalizers,verbs=update
// +kubebuilder:rbac:groups=puller.corewire.io,resources=pullpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

// Reconcile moves the cluster state closer to the desired state for a CachedImage.
func (r *CachedImageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// 1. Fetch CachedImage
	ci := &pullerv1alpha1.CachedImage{}
	if err := r.Get(ctx, req.NamespacedName, ci); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 2. List nodes matching nodeSelector
	nodeList := &corev1.NodeList{}
	listOpts := &client.ListOptions{}
	if len(ci.Spec.NodeSelector) > 0 {
		listOpts.LabelSelector = labels.SelectorFromSet(ci.Spec.NodeSelector)
	}
	if err := r.List(ctx, nodeList, listOpts); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing nodes: %w", err)
	}

	// 3. Filter nodes by tolerations
	targetNodes := filterNodesByTolerations(nodeList.Items, ci.Spec.Tolerations)

	// 4. Fetch referenced PullPolicy
	var policy *pullerv1alpha1.PullPolicy
	if ci.Spec.PolicyRef != nil {
		policy = &pullerv1alpha1.PullPolicy{}
		policyKey := client.ObjectKey{Name: ci.Spec.PolicyRef.Name}
		if err := r.Get(ctx, policyKey, policy); err != nil {
			if !errors.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("fetching PullPolicy: %w", err)
			}
			log.Info("referenced PullPolicy not found, using defaults", "policy", ci.Spec.PolicyRef.Name)
			policy = nil
		}
	}

	// 5. List owned Pods
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.MatchingLabels{
		podbuilder.LabelManagedBy:   podbuilder.LabelManagedByValue,
		podbuilder.LabelCachedImage: ci.Name,
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing owned pods: %w", err)
	}

	// 6. Build per-node state map
	type nodeState struct {
		pod    *corev1.Pod
		ready  bool
		failed bool
	}
	stateMap := make(map[string]*nodeState, len(targetNodes))
	for i := range targetNodes {
		stateMap[targetNodes[i].Name] = &nodeState{}
	}

	for i := range podList.Items {
		pod := &podList.Items[i]
		nodeName := pod.Labels[podbuilder.LabelNode]
		state, exists := stateMap[nodeName]
		if !exists {
			// Pod for node no longer in target set — delete it
			if err := r.Delete(ctx, pod); client.IgnoreNotFound(err) != nil {
				log.Error(err, "deleting orphan pod", "pod", pod.Name)
			}
			continue
		}
		state.pod = pod
	}

	// 7-8. Process pod states
	var nodesReady int32
	var requeueNeeded bool
	now := metav1.Now()

	for nodeName, state := range stateMap {
		if state.pod == nil {
			continue
		}

		switch state.pod.Status.Phase {
		case corev1.PodSucceeded:
			// Mark ready, cleanup pod
			state.ready = true
			nodesReady++
			if err := r.Delete(ctx, state.pod); client.IgnoreNotFound(err) != nil {
				log.Error(err, "deleting succeeded pod", "pod", state.pod.Name, "node", nodeName)
			}
		case corev1.PodFailed:
			// Record failure, cleanup pod
			state.failed = true
			log.Info("puller pod failed", "pod", state.pod.Name, "node", nodeName)
			if err := r.Delete(ctx, state.pod); client.IgnoreNotFound(err) != nil {
				log.Error(err, "deleting failed pod", "pod", state.pod.Name, "node", nodeName)
			}
		case corev1.PodRunning, corev1.PodPending:
			// Still in progress
			requeueNeeded = true
		}
	}

	// 9-10. For nodes needing pulls, check pacing and create pods
	var requeueAfter time.Duration
	for nodeName, state := range stateMap {
		if state.ready || state.pod != nil {
			continue
		}

		// Check pacing
		decision, err := r.PacingEngine.CanStartPull(ctx, policy, ci.Name)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("checking pacing: %w", err)
		}

		if !decision.Allowed {
			if decision.RequeueIn > requeueAfter {
				requeueAfter = decision.RequeueIn
			}
			requeueNeeded = true
			continue
		}

		// Create puller pod
		pod, err := podbuilder.BuildPullerPod(ci, nodeName, r.Scheme)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("building puller pod: %w", err)
		}

		if err := r.Create(ctx, pod); err != nil {
			if !errors.IsAlreadyExists(err) {
				return ctrl.Result{}, fmt.Errorf("creating puller pod: %w", err)
			}
		} else {
			log.Info("created puller pod", "pod", pod.Name, "node", nodeName, "image", ci.Spec.Image)
		}

		requeueNeeded = true
		break // Create one pod at a time, respecting pacing
	}

	// 11. Update status
	nodesTargeted := int32(len(targetNodes))
	phase := "Pending"
	if nodesReady == nodesTargeted && nodesTargeted > 0 {
		phase = "Ready"
	} else if nodesReady > 0 {
		phase = "Pulling"
	}

	// Check for degraded state (any failed nodes without ready state)
	for _, state := range stateMap {
		if state.failed && !state.ready {
			phase = "Degraded"
			break
		}
	}

	ci.Status.ObservedGeneration = ci.Generation
	ci.Status.NodesTargeted = nodesTargeted
	ci.Status.NodesReady = nodesReady
	ci.Status.Phase = phase

	if nodesReady > 0 {
		ci.Status.LastPulledAt = &now
	}

	readyCondition := metav1.Condition{
		Type:               conditionTypeReady,
		ObservedGeneration: ci.Generation,
		LastTransitionTime: now,
	}
	if phase == "Ready" {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "AllNodesCached"
		readyCondition.Message = fmt.Sprintf("Image cached on all %d target nodes", nodesTargeted)
	} else {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "InProgress"
		readyCondition.Message = fmt.Sprintf("%d/%d nodes ready", nodesReady, nodesTargeted)
	}
	meta.SetStatusCondition(&ci.Status.Conditions, readyCondition)

	if err := r.Status().Update(ctx, ci); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}

	// 12. Determine requeue
	if requeueNeeded {
		if requeueAfter == 0 {
			requeueAfter = 5 * time.Second
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

// filterNodesByTolerations returns nodes whose taints are tolerated.
func filterNodesByTolerations(nodes []corev1.Node, tolerations []corev1.Toleration) []corev1.Node {
	if len(tolerations) == 0 {
		// If no tolerations, only accept nodes without NoSchedule/NoExecute taints
		var result []corev1.Node
		for i := range nodes {
			if !hasUntoleratableTaints(nodes[i].Spec.Taints) {
				result = append(result, nodes[i])
			}
		}
		return result
	}

	var result []corev1.Node
	for i := range nodes {
		if allTaintsTolerated(nodes[i].Spec.Taints, tolerations) {
			result = append(result, nodes[i])
		}
	}
	return result
}

// hasUntoleratableTaints checks if any taint prevents scheduling.
func hasUntoleratableTaints(taints []corev1.Taint) bool {
	for _, taint := range taints {
		if taint.Effect == corev1.TaintEffectNoSchedule || taint.Effect == corev1.TaintEffectNoExecute {
			return true
		}
	}
	return false
}

// allTaintsTolerated checks if all NoSchedule/NoExecute taints are tolerated.
func allTaintsTolerated(taints []corev1.Taint, tolerations []corev1.Toleration) bool {
	for _, taint := range taints {
		if taint.Effect != corev1.TaintEffectNoSchedule && taint.Effect != corev1.TaintEffectNoExecute {
			continue
		}
		if !taintTolerated(taint, tolerations) {
			return false
		}
	}
	return true
}

// taintTolerated checks if a single taint is tolerated by any toleration.
func taintTolerated(taint corev1.Taint, tolerations []corev1.Toleration) bool {
	for _, toleration := range tolerations {
		if toleration.Operator == corev1.TolerationOpExists {
			if toleration.Key == "" {
				return true // Tolerates everything
			}
			if toleration.Key == taint.Key {
				if toleration.Effect == "" || toleration.Effect == taint.Effect {
					return true
				}
			}
		}
		if toleration.Operator == corev1.TolerationOpEqual || toleration.Operator == "" {
			if toleration.Key == taint.Key && toleration.Value == taint.Value {
				if toleration.Effect == "" || toleration.Effect == taint.Effect {
					return true
				}
			}
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *CachedImageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pullerv1alpha1.CachedImage{}).
		Owns(&corev1.Pod{}).
		Named("cachedimage").
		Complete(r)
}
