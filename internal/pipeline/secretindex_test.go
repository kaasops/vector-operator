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

package pipeline

import (
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

func namesOf(list []types.NamespacedName) []string {
	out := make([]string, 0, len(list))
	for _, n := range list {
		out = append(out, n.String())
	}
	sort.Strings(out)
	return out
}

func TestSecretIndexPipelinesForEmpty(t *testing.T) {
	si := NewSecretIndex()
	require.Empty(t, si.PipelinesFor(types.NamespacedName{Namespace: "ns", Name: "sec"}))
}

func TestSecretIndexSetAndPipelinesFor(t *testing.T) {
	si := NewSecretIndex()
	p1 := types.NamespacedName{Namespace: "ns", Name: "p1"}
	p2 := types.NamespacedName{Namespace: "ns", Name: "p2"}
	sec := types.NamespacedName{Namespace: "ns", Name: "sec"}

	si.Set(p1, []types.NamespacedName{sec})
	si.Set(p2, []types.NamespacedName{sec})

	require.Equal(t, []string{"ns/p1", "ns/p2"}, namesOf(si.PipelinesFor(sec)))
}

func TestSecretIndexSetReplacesFullSet(t *testing.T) {
	si := NewSecretIndex()
	p := types.NamespacedName{Namespace: "ns", Name: "p"}
	secA := types.NamespacedName{Namespace: "ns", Name: "secA"}
	secB := types.NamespacedName{Namespace: "ns", Name: "secB"}

	si.Set(p, []types.NamespacedName{secA})
	require.Equal(t, []string{"ns/p"}, namesOf(si.PipelinesFor(secA)))

	// Replacing with a different set must drop the stale link to secA.
	si.Set(p, []types.NamespacedName{secB})
	require.Empty(t, si.PipelinesFor(secA))
	require.Equal(t, []string{"ns/p"}, namesOf(si.PipelinesFor(secB)))
}

func TestSecretIndexSetEmptyRemovesPipelineEntirely(t *testing.T) {
	si := NewSecretIndex()
	p := types.NamespacedName{Namespace: "ns", Name: "p"}
	sec := types.NamespacedName{Namespace: "ns", Name: "sec"}

	si.Set(p, []types.NamespacedName{sec})
	require.NotEmpty(t, si.PipelinesFor(sec))

	si.Set(p, nil)
	require.Empty(t, si.PipelinesFor(sec))
}

func TestSecretIndexSetSharedSecretKeepsOtherPipeline(t *testing.T) {
	si := NewSecretIndex()
	p1 := types.NamespacedName{Namespace: "ns", Name: "p1"}
	p2 := types.NamespacedName{Namespace: "ns", Name: "p2"}
	sec := types.NamespacedName{Namespace: "ns", Name: "sec"}

	si.Set(p1, []types.NamespacedName{sec})
	si.Set(p2, []types.NamespacedName{sec})

	// Removing p1's link must not affect p2's.
	si.Set(p1, nil)
	require.Equal(t, []string{"ns/p2"}, namesOf(si.PipelinesFor(sec)))
}

func TestSecretIndexClusterScopedPipelineUsesEmptyNamespace(t *testing.T) {
	si := NewSecretIndex()
	cvp := types.NamespacedName{Namespace: "", Name: "cvp"}
	sec := types.NamespacedName{Namespace: "ns", Name: "sec"}

	si.Set(cvp, []types.NamespacedName{sec})
	require.Equal(t, []string{"/cvp"}, namesOf(si.PipelinesFor(sec)))
}

func TestSecretIndexConcurrentAccess(t *testing.T) {
	si := NewSecretIndex()
	sec := types.NamespacedName{Namespace: "ns", Name: "sec"}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			p := types.NamespacedName{Namespace: "ns", Name: "p"}
			si.Set(p, []types.NamespacedName{sec})
		}(i)
		go func() {
			defer wg.Done()
			si.PipelinesFor(sec)
		}()
	}
	wg.Wait()
}
