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

package main

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// Under a continuous stream of pipeline events arriving faster than the debounce
// delay, reconcileWithDelay must still flush queued agent reconciles. If it resets
// its timer on every event it starves — agents never get reconciled and their config
// secret is never (re)built, which is the e2e flake this reproduces deterministically.
func TestReconcileWithDelayFlushesUnderContinuousLoad(t *testing.T) {
	in := make(chan event.GenericEvent, 100)
	out := make(chan event.GenericEvent, 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	delay := 50 * time.Millisecond
	go reconcileWithDelay(ctx, in, out, delay)

	obj := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "agent"}}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		tk := time.NewTicker(delay / 5) // events arrive faster than the debounce window
		defer tk.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				select {
				case in <- event.GenericEvent{Object: obj}:
				case <-stop:
					return
				}
			}
		}
	}()

	select {
	case <-out:
		// flushed despite continuous load — correct
	case <-time.After(delay * 10):
		t.Fatal("reconcileWithDelay never flushed under continuous load (debounce starvation)")
	}
}
