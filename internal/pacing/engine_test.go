package pacing

import (
	"context"
	"testing"
	"time"

	v1alpha1 "github.com/Breee/puller/api/v1alpha1"
	"github.com/Breee/puller/internal/podbuilder"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = v1alpha1.AddToScheme(s)
	return s
}

func TestCanStartPull(t *testing.T) {
	tests := []struct {
		name        string
		policy      *v1alpha1.PullPolicy
		activePods  []corev1.Pod
		wantAllowed bool
		wantRequeue bool
	}{
		{
			name:        "allows when no active pulls exist",
			policy:      nil,
			activePods:  nil,
			wantAllowed: true,
			wantRequeue: false,
		},
		{
			name: "denies when maxConcurrentNodes reached",
			policy: &v1alpha1.PullPolicy{
				Spec: v1alpha1.PullPolicySpec{
					MaxConcurrentNodes:   1,
					MinDelayBetweenPulls: metav1.Duration{Duration: 10 * time.Second},
				},
			},
			activePods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "puller-test-1",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Second)),
						Labels: map[string]string{
							podbuilder.LabelManagedBy: podbuilder.LabelManagedByValue,
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
			},
			wantAllowed: false,
			wantRequeue: true,
		},
		{
			name: "allows when at boundary (maxConcurrentNodes - 1 active)",
			policy: &v1alpha1.PullPolicy{
				Spec: v1alpha1.PullPolicySpec{
					MaxConcurrentNodes:   2,
					MinDelayBetweenPulls: metav1.Duration{Duration: 1 * time.Second},
				},
			},
			activePods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "puller-test-1",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Second)),
						Labels: map[string]string{
							podbuilder.LabelManagedBy: podbuilder.LabelManagedByValue,
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
			},
			wantAllowed: true,
			wantRequeue: false,
		},
		{
			name: "denies when minDelayBetweenPulls not elapsed",
			policy: &v1alpha1.PullPolicy{
				Spec: v1alpha1.PullPolicySpec{
					MaxConcurrentNodes:   5,
					MinDelayBetweenPulls: metav1.Duration{Duration: 60 * time.Second},
				},
			},
			activePods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "puller-test-1",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Second)),
						Labels: map[string]string{
							podbuilder.LabelManagedBy: podbuilder.LabelManagedByValue,
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodPending},
				},
			},
			wantAllowed: false,
			wantRequeue: true,
		},
		{
			name:   "uses defaults when nil policy",
			policy: nil,
			activePods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "puller-test-1",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Second)),
						Labels: map[string]string{
							podbuilder.LabelManagedBy: podbuilder.LabelManagedByValue,
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
			},
			wantAllowed: false,
			wantRequeue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := testScheme()

			objs := make([]runtime.Object, 0, len(tt.activePods))
			for i := range tt.activePods {
				tt.activePods[i].Namespace = "puller-system"
				objs = append(objs, &tt.activePods[i])
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objs...).
				Build()

			engine := NewEngine(fakeClient, "puller-system")
			decision, err := engine.CanStartPull(context.Background(), tt.policy, "test-image")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if decision.Allowed != tt.wantAllowed {
				t.Errorf("Allowed = %v, want %v", decision.Allowed, tt.wantAllowed)
			}

			if tt.wantRequeue && decision.RequeueIn == 0 {
				t.Error("expected non-zero RequeueIn")
			}
			if !tt.wantRequeue && decision.RequeueIn != 0 {
				t.Errorf("expected zero RequeueIn, got %v", decision.RequeueIn)
			}
		})
	}
}
