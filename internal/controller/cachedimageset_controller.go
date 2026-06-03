/*
Copyright (c) 2026 Breee

SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dropv1alpha1 "github.com/corewire/drop/api/v1alpha1"
)

const labelImageSet = "drop.corewire.io/imageset"

// CachedImageSetReconciler reconciles a CachedImageSet object
type CachedImageSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=drop.corewire.io,resources=cachedimagesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=drop.corewire.io,resources=cachedimagesets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=drop.corewire.io,resources=cachedimagesets/finalizers,verbs=update
// +kubebuilder:rbac:groups=drop.corewire.io,resources=discoverypolicies,verbs=get;list;watch

// Reconcile manages child CachedImage resources for a CachedImageSet.
func (r *CachedImageSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// 1. Fetch CachedImageSet
	imageSet := &dropv1alpha1.CachedImageSet{}
	if err := r.Get(ctx, req.NamespacedName, imageSet); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 2. Build desired image list
	desiredImages := r.buildDesiredImages(ctx, imageSet)

	// 3. List existing child CachedImage resources
	existingChildren := &dropv1alpha1.CachedImageList{}
	if err := r.List(ctx, existingChildren, client.MatchingLabels{
		labelImageSet: imageSet.Name,
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing children: %w", err)
	}

	// Build map of existing children by image ref
	existingMap := make(map[string]*dropv1alpha1.CachedImage, len(existingChildren.Items))
	for i := range existingChildren.Items {
		child := &existingChildren.Items[i]
		ref := buildChildImageRef(child)
		existingMap[ref] = child
	}

	// 4. Diff: create new, delete removed
	desiredSet := make(map[string]dropv1alpha1.ImageEntry, len(desiredImages))
	for _, img := range desiredImages {
		ref := buildEntryRef(img)
		desiredSet[ref] = img
	}

	// Delete children that are no longer desired
	for ref, child := range existingMap {
		if _, wanted := desiredSet[ref]; !wanted {
			log.Info("deleting child CachedImage", "name", child.Name, "image", ref)
			if err := r.Delete(ctx, child); client.IgnoreNotFound(err) != nil {
				return ctrl.Result{}, fmt.Errorf("deleting child: %w", err)
			}
		}
	}

	// Create children that don't exist yet
	for ref, img := range desiredSet {
		if _, exists := existingMap[ref]; exists {
			continue
		}

		child := r.buildChildCachedImage(imageSet, img)
		if err := controllerutil.SetControllerReference(imageSet, child, r.Scheme); err != nil {
			return ctrl.Result{}, fmt.Errorf("setting owner reference: %w", err)
		}

		log.Info("creating child CachedImage", "name", child.Name, "image", ref)
		if err := r.Create(ctx, child); err != nil {
			if !errors.IsAlreadyExists(err) {
				return ctrl.Result{}, fmt.Errorf("creating child: %w", err)
			}
		}
	}

	// 5. Update status
	// Re-list children after mutations
	patch := client.MergeFrom(imageSet.DeepCopy())
	if err := r.List(ctx, existingChildren, client.MatchingLabels{
		"drop.corewire.io/imageset": imageSet.Name,
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("re-listing children: %w", err)
	}

	var imagesReady int32
	var worstReason, worstMessage string
	var hasDegraded bool
	for i := range existingChildren.Items {
		child := &existingChildren.Items[i]
		switch child.Status.Phase {
		case phaseReady:
			imagesReady++
		case phaseDegraded:
			hasDegraded = true
			// Extract the child's failure reason for propagation
			for _, c := range child.Status.Conditions {
				if c.Type == conditionTypeReady && c.Status == metav1.ConditionFalse && c.Reason != "InProgress" {
					worstReason = c.Reason
					worstMessage = c.Message
				}
			}
		}
	}

	imageSet.Status.ObservedGeneration = imageSet.Generation
	imageSet.Status.ImagesManaged = int32(len(existingChildren.Items))
	imageSet.Status.ImagesReady = imagesReady

	if imagesReady == int32(len(desiredImages)) && len(desiredImages) > 0 {
		imageSet.Status.Phase = phaseReady
	} else if hasDegraded {
		imageSet.Status.Phase = phaseDegraded
	} else {
		imageSet.Status.Phase = phasePending
	}

	now := metav1.Now()
	readyCondition := metav1.Condition{
		Type:               conditionTypeReady,
		ObservedGeneration: imageSet.Generation,
		LastTransitionTime: now,
	}
	switch {
	case imageSet.Status.Phase == phaseReady:
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "Ready"
		readyCondition.Message = fmt.Sprintf("All %d images are cached", imagesReady)
	case hasDegraded && worstReason != "":
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "Degraded"
		readyCondition.Message = fmt.Sprintf("%d/%d images cached, failing: %s", imagesReady, len(desiredImages), worstReason)
		if worstMessage != "" {
			readyCondition.Message = fmt.Sprintf("%d/%d images cached: %s", imagesReady, len(desiredImages), worstMessage)
		}
	default:
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "Progressing"
		readyCondition.Message = fmt.Sprintf("%d/%d images cached", imagesReady, len(desiredImages))
	}
	meta.SetStatusCondition(&imageSet.Status.Conditions, readyCondition)

	if err := r.Status().Patch(ctx, imageSet, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("patching status: %w", err)
	}

	return ctrl.Result{}, nil
}

// buildDesiredImages constructs the desired image list from static images and discovery.
func (r *CachedImageSetReconciler) buildDesiredImages(ctx context.Context, imageSet *dropv1alpha1.CachedImageSet) []dropv1alpha1.ImageEntry {
	var desired []dropv1alpha1.ImageEntry

	// Static images
	desired = append(desired, imageSet.Spec.Images...)

	// Discovery policy images
	if imageSet.Spec.DiscoveryPolicyRef != nil {
		dp := &dropv1alpha1.DiscoveryPolicy{}
		key := client.ObjectKey{Name: imageSet.Spec.DiscoveryPolicyRef.Name}
		if err := r.Get(ctx, key, dp); err == nil {
			for _, discovered := range dp.Status.DiscoveredImages {
				entry := parseImageRef(discovered.Image)
				desired = append(desired, entry)
			}
		}
	}

	return desired
}

// parseImageRef splits a full image reference into ImageEntry.
func parseImageRef(ref string) dropv1alpha1.ImageEntry {
	if idx := strings.Index(ref, "@"); idx != -1 {
		return dropv1alpha1.ImageEntry{
			Image:  ref[:idx],
			Digest: ref[idx+1:],
		}
	}
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		// Ensure it's a tag separator and not a port
		afterColon := ref[idx+1:]
		if !strings.Contains(afterColon, "/") {
			return dropv1alpha1.ImageEntry{
				Image: ref[:idx],
				Tag:   afterColon,
			}
		}
	}
	return dropv1alpha1.ImageEntry{Image: ref}
}

// buildChildCachedImage creates a CachedImage spec from an ImageEntry.
func (r *CachedImageSetReconciler) buildChildCachedImage(parent *dropv1alpha1.CachedImageSet, img dropv1alpha1.ImageEntry) *dropv1alpha1.CachedImage {
	name := sanitizeName(fmt.Sprintf("%s-%s-%s", parent.Name, imageName(img.Image), img.Tag))
	if img.Digest != "" {
		name = sanitizeName(fmt.Sprintf("%s-%s-digest", parent.Name, imageName(img.Image)))
	}

	child := &dropv1alpha1.CachedImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"drop.corewire.io/imageset": parent.Name,
			},
		},
		Spec: dropv1alpha1.CachedImageSpec{
			Image:            img.Image,
			Tag:              img.Tag,
			Digest:           img.Digest,
			ImagePullPolicy:  parent.Spec.ImagePullPolicy,
			ImagePullSecrets: parent.Spec.ImagePullSecrets,
			NodeSelector:     parent.Spec.NodeSelector,
			Tolerations:      parent.Spec.Tolerations,
			PolicyRef:        parent.Spec.PolicyRef,
		},
	}

	return child
}

// buildChildImageRef creates a comparable ref from a CachedImage.
func buildChildImageRef(ci *dropv1alpha1.CachedImage) string {
	return buildEntryRef(dropv1alpha1.ImageEntry{
		Image:  ci.Spec.Image,
		Tag:    ci.Spec.Tag,
		Digest: ci.Spec.Digest,
	})
}

// buildEntryRef creates a comparable ref from an ImageEntry.
func buildEntryRef(entry dropv1alpha1.ImageEntry) string {
	if entry.Digest != "" {
		return fmt.Sprintf("%s@%s", entry.Image, entry.Digest)
	}
	tag := entry.Tag
	if tag == "" {
		tag = "latest"
	}
	return fmt.Sprintf("%s:%s", entry.Image, tag)
}

// imageName extracts the short name from a full image reference.
func imageName(image string) string {
	parts := strings.Split(image, "/")
	return parts[len(parts)-1]
}

// sanitizeName ensures the name is a valid k8s resource name.
func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, "_", "-")
	if len(name) > 253 {
		name = name[:253]
	}
	return name
}

// mapDiscoveryToSets maps DiscoveryPolicy changes to CachedImageSets that reference them.
func (r *CachedImageSetReconciler) mapDiscoveryToSets(ctx context.Context, obj client.Object) []reconcile.Request {
	dp, ok := obj.(*dropv1alpha1.DiscoveryPolicy)
	if !ok {
		return nil
	}

	setList := &dropv1alpha1.CachedImageSetList{}
	if err := r.List(ctx, setList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for i := range setList.Items {
		set := &setList.Items[i]
		if set.Spec.DiscoveryPolicyRef != nil && set.Spec.DiscoveryPolicyRef.Name == dp.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: set.Name},
			})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *CachedImageSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dropv1alpha1.CachedImageSet{}).
		Owns(&dropv1alpha1.CachedImage{}).
		Watches(&dropv1alpha1.DiscoveryPolicy{}, handler.EnqueueRequestsFromMapFunc(r.mapDiscoveryToSets)).
		Named("cachedimageset").
		Complete(r)
}
