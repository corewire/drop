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
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	pullerv1alpha1 "github.com/Breee/puller/api/v1alpha1"
	pullermetrics "github.com/Breee/puller/internal/metrics"
	"github.com/Breee/puller/internal/pacing"
	"github.com/Breee/puller/internal/podbuilder"
)

const (
	conditionTypeReady = "Ready"
	phasePending       = "Pending"
	phaseReady         = "Ready"
	phasePulling       = "Pulling"
	phaseDegraded      = "Degraded"
)

// CachedImageReconciler reconciles a CachedImage object
type CachedImageReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	PacingEngine *pacing.Engine
	Recorder     record.EventRecorder
}

// +kubebuilder:rbac:groups=puller.corewire.io,resources=cachedimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=puller.corewire.io,resources=cachedimages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=puller.corewire.io,resources=cachedimages/finalizers,verbs=update
// +kubebuilder:rbac:groups=puller.corewire.io,resources=pullpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// nodeState tracks the pull state for a single node.
type nodeState struct {
	pod    *corev1.Pod
	ready  bool
	failed bool
}

// Reconcile moves the cluster state closer to the desired state for a CachedImage.
func (r *CachedImageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// 1. Fetch CachedImage
	ci := &pullerv1alpha1.CachedImage{}
	if err := r.Get(ctx, req.NamespacedName, ci); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 2-3. Resolve target nodes
	targetNodes, err := r.resolveTargetNodes(ctx, ci)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 4. Fetch referenced PullPolicy
	policy, err := r.fetchPullPolicy(ctx, ci)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 5-6. Build per-node state from owned Pods
	stateMap, err := r.buildNodeStateMap(ctx, ci, targetNodes)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 7-8. Process pod states
	nodesReady, requeueNeeded := r.processPodStates(ctx, ci, stateMap)

	// 9-10. Schedule pulls for nodes that need them
	requeueAfter, pullRequeue, err := r.schedulePulls(ctx, ci, policy, stateMap)
	if err != nil {
		return ctrl.Result{}, err
	}
	requeueNeeded = requeueNeeded || pullRequeue

	// 11. Update status
	nodesTargeted := int32(len(targetNodes))
	now := metav1.Now()
	r.updateCachedImageStatus(ci, stateMap, nodesTargeted, nodesReady, now)

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

// resolveTargetNodes lists and filters nodes matching the CachedImage spec.
func (r *CachedImageReconciler) resolveTargetNodes(ctx context.Context, ci *pullerv1alpha1.CachedImage) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	listOpts := &client.ListOptions{}
	if len(ci.Spec.NodeSelector) > 0 {
		listOpts.LabelSelector = labels.SelectorFromSet(ci.Spec.NodeSelector)
	}
	if err := r.List(ctx, nodeList, listOpts); err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}
	return filterNodesByTolerations(nodeList.Items, ci.Spec.Tolerations), nil
}

// fetchPullPolicy retrieves the referenced PullPolicy, if any.
func (r *CachedImageReconciler) fetchPullPolicy(ctx context.Context, ci *pullerv1alpha1.CachedImage) (*pullerv1alpha1.PullPolicy, error) {
	if ci.Spec.PolicyRef == nil {
		return nil, nil
	}
	log := logf.FromContext(ctx)
	policy := &pullerv1alpha1.PullPolicy{}
	policyKey := client.ObjectKey{Name: ci.Spec.PolicyRef.Name}
	if err := r.Get(ctx, policyKey, policy); err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("fetching PullPolicy: %w", err)
		}
		log.Info("referenced PullPolicy not found, using defaults", "policy", ci.Spec.PolicyRef.Name)
		return nil, nil
	}
	return policy, nil
}

// buildNodeStateMap creates the per-node state map from owned Pods.
func (r *CachedImageReconciler) buildNodeStateMap(ctx context.Context, ci *pullerv1alpha1.CachedImage, targetNodes []corev1.Node) (map[string]*nodeState, error) {
	log := logf.FromContext(ctx)

	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.MatchingLabels{
		podbuilder.LabelManagedBy:   podbuilder.LabelManagedByValue,
		podbuilder.LabelCachedImage: ci.Name,
	}); err != nil {
		return nil, fmt.Errorf("listing owned pods: %w", err)
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
			if err := r.Delete(ctx, pod); client.IgnoreNotFound(err) != nil {
				log.Error(err, "deleting orphan pod", "pod", pod.Name)
			}
			continue
		}
		state.pod = pod
	}

	return stateMap, nil
}

// processPodStates evaluates completed/failed/running pods and returns ready count.
func (r *CachedImageReconciler) processPodStates(ctx context.Context, ci *pullerv1alpha1.CachedImage, stateMap map[string]*nodeState) (int32, bool) {
	log := logf.FromContext(ctx)
	var nodesReady int32
	var requeueNeeded bool

	for nodeName, state := range stateMap {
		if state.pod == nil {
			continue
		}

		switch state.pod.Status.Phase {
		case corev1.PodSucceeded:
			state.ready = true
			nodesReady++
			pullermetrics.ImagesCachedTotal.WithLabelValues(ci.Spec.Image, nodeName).Inc()
			r.Recorder.Eventf(ci, corev1.EventTypeNormal, "PullSucceeded", "Image %s cached on node %s", ci.Spec.Image, nodeName)
			if err := r.Delete(ctx, state.pod); client.IgnoreNotFound(err) != nil {
				log.Error(err, "deleting succeeded pod", "pod", state.pod.Name, "node", nodeName)
			}
		case corev1.PodFailed:
			state.failed = true
			pullermetrics.PullErrorsTotal.WithLabelValues(ci.Spec.Image, nodeName).Inc()
			r.Recorder.Eventf(ci, corev1.EventTypeWarning, "PullFailed", "Failed to pull image %s on node %s", ci.Spec.Image, nodeName)
			log.Info("puller pod failed", "pod", state.pod.Name, "node", nodeName)
			if err := r.Delete(ctx, state.pod); client.IgnoreNotFound(err) != nil {
				log.Error(err, "deleting failed pod", "pod", state.pod.Name, "node", nodeName)
			}
		case corev1.PodRunning, corev1.PodPending:
			requeueNeeded = true
		}
	}

	return nodesReady, requeueNeeded
}

// schedulePulls creates puller pods for nodes that need them, respecting pacing.
func (r *CachedImageReconciler) schedulePulls(ctx context.Context, ci *pullerv1alpha1.CachedImage, policy *pullerv1alpha1.PullPolicy, stateMap map[string]*nodeState) (time.Duration, bool, error) {
	log := logf.FromContext(ctx)
	var requeueAfter time.Duration
	var requeueNeeded bool

	for nodeName, state := range stateMap {
		if state.ready || state.pod != nil {
			continue
		}

		decision, err := r.PacingEngine.CanStartPull(ctx, policy, ci.Name)
		if err != nil {
			return 0, false, fmt.Errorf("checking pacing: %w", err)
		}

		if !decision.Allowed {
			if decision.RequeueIn > requeueAfter {
				requeueAfter = decision.RequeueIn
			}
			requeueNeeded = true
			continue
		}

		pod, err := podbuilder.BuildPullerPod(ci, nodeName, r.Scheme)
		if err != nil {
			return 0, false, fmt.Errorf("building puller pod: %w", err)
		}

		if err := r.Create(ctx, pod); err != nil {
			if !errors.IsAlreadyExists(err) {
				return 0, false, fmt.Errorf("creating puller pod: %w", err)
			}
		} else {
			pullermetrics.ActivePulls.Inc()
			r.Recorder.Eventf(ci, corev1.EventTypeNormal, "PullStarted", "Started pulling image %s on node %s", ci.Spec.Image, nodeName)
			log.Info("created puller pod", "pod", pod.Name, "node", nodeName, "image", ci.Spec.Image)
		}

		requeueNeeded = true
		break // Create one pod at a time, respecting pacing
	}

	return requeueAfter, requeueNeeded, nil
}

// updateCachedImageStatus computes and sets the status fields on the CachedImage.
func (r *CachedImageReconciler) updateCachedImageStatus(ci *pullerv1alpha1.CachedImage, stateMap map[string]*nodeState, nodesTargeted, nodesReady int32, now metav1.Time) {
	phase := phasePending
	if nodesReady == nodesTargeted && nodesTargeted > 0 {
		phase = phaseReady
	} else if nodesReady > 0 {
		phase = phasePulling
	}

	for _, state := range stateMap {
		if state.failed && !state.ready {
			phase = phaseDegraded
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
	if phase == phaseReady {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "AllNodesCached"
		readyCondition.Message = fmt.Sprintf("Image cached on all %d target nodes", nodesTargeted)
	} else {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "InProgress"
		readyCondition.Message = fmt.Sprintf("%d/%d nodes ready", nodesReady, nodesTargeted)
	}
	meta.SetStatusCondition(&ci.Status.Conditions, readyCondition)
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
