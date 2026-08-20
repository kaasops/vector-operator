/*
Copyright 2024.

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

package configcheck

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	k8stesting "k8s.io/client-go/testing"
)

type checkResult struct {
	reason string
	err    error
}

// startGetCheckResult runs getCheckResult against a fake watcher the test controls.
// The budget is short so a starved wait surfaces as ErrConfigcheckTimeout quickly.
// The watcher is race-free: getCheckResult stops it on return, and a test that keeps
// feeding events would otherwise send on a closed channel.
func startGetCheckResult(t *testing.T, budget time.Duration) (*watch.RaceFreeFakeWatcher, *corev1.Pod, chan checkResult) {
	t.Helper()
	cs := k8sfake.NewSimpleClientset()
	fw := watch.NewRaceFreeFake()
	cs.PrependWatchReactor("pods", k8stesting.DefaultWatchReactor(fw, nil))

	cc := &ConfigCheck{ClientSet: cs, Namespace: "ns", ConfigCheckTimeout: budget}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "configcheck-x"}}

	res := make(chan checkResult, 1)
	go func() {
		reason, err := cc.getCheckResult(context.Background(), pod, time.Now().Add(budget))
		res <- checkResult{reason, err}
	}()
	return fw, pod, res
}

func waitResult(t *testing.T, res chan checkResult, within time.Duration) checkResult {
	t.Helper()
	select {
	case r := <-res:
		return r
	case <-time.After(within):
		t.Fatal("getCheckResult did not return in time")
		return checkResult{}
	}
}

// A configcheck pod that can never start (e.g. env references a missing secret →
// CreateContainerConfigError) must fail the check immediately instead of holding
// the reconcile worker until ConfigCheckTimeout.
func TestGetCheckResultFailsFastOnUnstartablePod(t *testing.T) {
	fw, pod, res := startGetCheckResult(t, 5*time.Second)

	bad := pod.DeepCopy()
	bad.Status.Phase = corev1.PodPending
	bad.Status.ContainerStatuses = []corev1.ContainerStatus{{
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
			Reason:  "CreateContainerConfigError",
			Message: `secret "no-such-secret" not found`,
		}},
	}}
	fw.Modify(bad)

	r := waitResult(t, res, 2*time.Second) // well under the 5s timeout
	if !errors.Is(r.err, ErrValidation) {
		t.Fatalf("want ErrValidation, got err=%v reason=%q", r.err, r.reason)
	}
	if !strings.Contains(r.reason, "CreateContainerConfigError") {
		t.Fatalf("reason should carry the waiting reason, got %q", r.reason)
	}
}

// The API server seeds a resourceVersion-less watch with synthetic Added events; a pod
// that completed before the watch was established arrives as Added, not Modified, and
// must be treated as a result rather than ignored until timeout.
func TestGetCheckResultHandlesInitialAddedEvent(t *testing.T) {
	fw, pod, res := startGetCheckResult(t, 5*time.Second)

	done := pod.DeepCopy()
	done.Status.Phase = corev1.PodSucceeded
	fw.Add(done)

	r := waitResult(t, res, 2*time.Second)
	if r.err != nil || r.reason != "" {
		t.Fatalf("want success, got err=%v reason=%q", r.err, r.reason)
	}
}

// If the configcheck pod is deleted before completing (namespace teardown, manual
// cleanup), the check can never produce a result — bail out instead of waiting.
func TestGetCheckResultFailsWhenPodDeleted(t *testing.T) {
	fw, pod, res := startGetCheckResult(t, 5*time.Second)

	fw.Delete(pod.DeepCopy())

	r := waitResult(t, res, 2*time.Second)
	if r.err == nil {
		t.Fatalf("want error on deleted pod, got success (reason=%q)", r.reason)
	}
}

// A closed watch channel means we lost sight of the pod; reporting success would let
// an unvalidated config through. It must surface as an error.
func TestGetCheckResultErrsOnClosedWatchChannel(t *testing.T) {
	fw, _, res := startGetCheckResult(t, 5*time.Second)

	// give the watch loop a moment to start consuming, then close the stream
	time.Sleep(50 * time.Millisecond)
	fw.Stop()

	r := waitResult(t, res, 2*time.Second)
	if r.err == nil {
		t.Fatalf("want error on closed watch, got success (reason=%q)", r.reason)
	}
}

// ConfigCheckTimeout must bound the whole check, not the gap between pod events.
// A pod that keeps emitting status updates - image pull progress, repeated scheduling
// attempts - would otherwise extend the check indefinitely: it holds the reconcile
// worker, and it outlives the age window the orphan sweep relies on to tell a running
// check from a leftover.
func TestGetCheckResultTimeoutBoundsTheWholeCheck(t *testing.T) {
	const timeout = 300 * time.Millisecond
	fw, pod, res := startGetCheckResult(t, timeout)

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		tick := time.NewTicker(timeout / 6)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				pending := pod.DeepCopy()
				pending.Status.Phase = corev1.PodPending
				fw.Modify(pending)
			}
		}
	}()

	start := time.Now()
	r := waitResult(t, res, 3*time.Second)
	if !errors.Is(r.err, ErrConfigcheckTimeout) {
		t.Fatalf("want ErrConfigcheckTimeout, got err=%v reason=%q", r.err, r.reason)
	}
	if elapsed := time.Since(start); elapsed > 4*timeout {
		t.Fatalf("check ran for %v, the timeout must bound it at about %v", elapsed, timeout)
	}
}

// The budget starts in Run, before the configcheck Secret is even created, and
// getCheckResult only gets what is left of it. A deadline that already passed by the
// time the watch is established must end the check at once - the Secret is by then as
// old as the whole timeout, which is exactly when the orphan sweep may remove it.
func TestGetCheckResultEndsOnAnExpiredDeadline(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	fw := watch.NewRaceFreeFake()
	defer fw.Stop()
	cs.PrependWatchReactor("pods", k8stesting.DefaultWatchReactor(fw, nil))

	cc := &ConfigCheck{ClientSet: cs, Namespace: "ns", ConfigCheckTimeout: time.Hour}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "configcheck-x"}}

	res := make(chan checkResult, 1)
	go func() {
		reason, err := cc.getCheckResult(context.Background(), pod, time.Now().Add(-time.Second))
		res <- checkResult{reason, err}
	}()

	r := waitResult(t, res, 2*time.Second)
	if !errors.Is(r.err, ErrConfigcheckTimeout) {
		t.Fatalf("want ErrConfigcheckTimeout, got err=%v reason=%q", r.err, r.reason)
	}
}

// podWatchCtxRecorder captures the context Watch is called with. The fake clientset's
// reactors never see that context, and what matters here is exactly that the request
// establishing the watch carries the check budget: a hung request would otherwise sit
// outside both the budget context and the timer.
type podWatchCtxRecorder struct {
	kubernetes.Interface
	got chan context.Context
}

func (r *podWatchCtxRecorder) CoreV1() corev1client.CoreV1Interface {
	return &recordingCoreV1{r.Interface.CoreV1(), r}
}

type recordingCoreV1 struct {
	corev1client.CoreV1Interface
	rec *podWatchCtxRecorder
}

func (c *recordingCoreV1) Pods(namespace string) corev1client.PodInterface {
	return &recordingPods{c.CoreV1Interface.Pods(namespace), c.rec}
}

type recordingPods struct {
	corev1client.PodInterface
	rec *podWatchCtxRecorder
}

func (p *recordingPods) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	select {
	case p.rec.got <- ctx:
	default:
	}
	return p.PodInterface.Watch(ctx, opts)
}

// Establishing the watch is an HTTP request of its own, and it must be bounded by the
// same budget as the rest of the check: if it hangs, neither the timer (not created
// yet) nor the caller's ctx would end it.
func TestGetCheckResultBoundsTheWatchRequestByTheBudget(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	fw := watch.NewRaceFreeFake()
	defer fw.Stop()
	cs.PrependWatchReactor("pods", k8stesting.DefaultWatchReactor(fw, nil))
	rec := &podWatchCtxRecorder{Interface: cs, got: make(chan context.Context, 1)}

	cc := &ConfigCheck{ClientSet: rec, Namespace: "ns", ConfigCheckTimeout: time.Hour}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "configcheck-x"}}
	deadline := time.Now().Add(300 * time.Millisecond)

	res := make(chan checkResult, 1)
	go func() {
		reason, err := cc.getCheckResult(context.Background(), pod, deadline)
		res <- checkResult{reason, err}
	}()

	var watchCtx context.Context
	select {
	case watchCtx = <-rec.got:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch was never called")
	}

	got, ok := watchCtx.Deadline()
	if !ok {
		t.Fatal("the request establishing the watch must carry the check budget")
	}
	if got.After(deadline.Add(50 * time.Millisecond)) {
		t.Fatalf("watch deadline %v runs past the check budget %v", got, deadline)
	}

	waitResult(t, res, 3*time.Second)
}

// A cancelled context means the caller gave up, not that the config is valid.
// getCheckResult used to return ("", nil) there, which the pipeline reconciler reads
// as a passing check and turns into a success status - a config that was never
// validated would be published as validated.
func TestGetCheckResultDoesNotReportSuccessOnCancellation(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	fw := watch.NewRaceFreeFake()
	defer fw.Stop()
	cs.PrependWatchReactor("pods", k8stesting.DefaultWatchReactor(fw, nil))

	cc := &ConfigCheck{ClientSet: cs, Namespace: "ns", ConfigCheckTimeout: time.Hour}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "configcheck-x"}}

	ctx, cancel := context.WithCancel(context.Background())
	res := make(chan checkResult, 1)
	go func() {
		reason, err := cc.getCheckResult(ctx, pod, time.Now().Add(time.Hour))
		res <- checkResult{reason, err}
	}()

	cancel()

	r := waitResult(t, res, 2*time.Second)
	if r.err == nil {
		t.Fatalf("a cancelled check must not look like a passing one, got reason=%q err=nil", r.reason)
	}
	if !errors.Is(r.err, context.Canceled) {
		t.Fatalf("want an error carrying context.Canceled, got %v", r.err)
	}
}
