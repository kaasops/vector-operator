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
	"testing"
	"time"
)

// TestTimeAfterLeakAllocations demonstrates memory leak when using time.After() in a loop.
// Each call to time.After() allocates ~280 bytes that won't be freed until the timer fires.
//
// This test shows the problem with the current implementation in getCheckResult():
// - time.After() in select loop: 283 B/op, 3 allocs/op
// - time.NewTimer with Stop/Reset: 0 B/op, 0 allocs/op
//
// In a long-running operator with frequent watch events, this causes continuous memory growth.
func TestTimeAfterLeakAllocations(t *testing.T) {
	eventChan := make(chan struct{}, 1)
	timeout := 10 * time.Second

	// Measure allocations for time.After() pattern (problematic)
	leakyAllocs := testing.AllocsPerRun(100, func() {
		eventChan <- struct{}{}
		select {
		case <-eventChan:
		case <-time.After(timeout):
		}
	})

	// Measure allocations for time.NewTimer pattern (correct)
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	fixedAllocs := testing.AllocsPerRun(100, func() {
		eventChan <- struct{}{}
		select {
		case <-eventChan:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(timeout)
		case <-timer.C:
		}
	})

	t.Logf("time.After() allocations per op: %.1f", leakyAllocs)
	t.Logf("time.NewTimer allocations per op: %.1f", fixedAllocs)

	// time.After() should allocate ~3 objects per call
	if leakyAllocs < 2 {
		t.Errorf("Expected time.After() to allocate >= 2 objects per call, got %.1f", leakyAllocs)
	}

	// time.NewTimer with Stop/Reset should allocate 0 objects
	if fixedAllocs > 0.5 {
		t.Errorf("Expected time.NewTimer pattern to allocate ~0 objects per call, got %.1f", fixedAllocs)
	}

	// The leak: each iteration wastes ~280 bytes until timer fires
	// With 1000 events and 60s timeout, that's 280KB of leaked memory per minute
	t.Logf("LEAK IMPACT: %.0f allocations leaked per iteration", leakyAllocs-fixedAllocs)
	t.Logf("With 1000 events and 60s ConfigCheckTimeout: ~%.0f KB leaked until timers fire",
		leakyAllocs*280*1000/1024)
}

// BenchmarkTimeAfterLeak benchmarks memory allocation with leaky time.After()
func BenchmarkTimeAfterLeak(b *testing.B) {
	eventChan := make(chan struct{}, 1)
	timeout := 1 * time.Hour // Very long timeout

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eventChan <- struct{}{}
		select {
		case <-eventChan:
		case <-time.After(timeout):
		}
	}
}

// BenchmarkTimeAfterFixed benchmarks memory allocation with fixed timer pattern
func BenchmarkTimeAfterFixed(b *testing.B) {
	eventChan := make(chan struct{}, 1)
	timeout := 1 * time.Hour

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eventChan <- struct{}{}
		select {
		case <-eventChan:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(timeout)
		case <-timer.C:
		}
	}
}
