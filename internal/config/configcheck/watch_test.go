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
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type checkResult struct {
	reason string
	err    error
}

// startGetCheckResult runs getCheckResult against a fake watcher the test controls.
// The timeout is short so a starved wait surfaces as ErrConfigcheckTimeout quickly.
func startGetCheckResult(t *testing.T, timeout time.Duration) (*watch.FakeWatcher, *corev1.Pod, chan checkResult) {
	t.Helper()
	cs := k8sfake.NewSimpleClientset()
	fw := watch.NewFakeWithChanSize(4, false)
	cs.PrependWatchReactor("pods", k8stesting.DefaultWatchReactor(fw, nil))

	cc := &ConfigCheck{ClientSet: cs, Namespace: "ns", ConfigCheckTimeout: timeout}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "configcheck-x"}}

	res := make(chan checkResult, 1)
	go func() {
		reason, err := cc.getCheckResult(context.Background(), pod)
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
