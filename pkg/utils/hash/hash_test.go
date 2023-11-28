/*
Copyright 2022.

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

package hash_test

import (
	"testing"

	"github.com/kaasops/vector-operator/pkg/utils/hash"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	hashCase := func(bytes []byte, want uint32) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			t.Parallel()
			req := require.New(t)

			result := hash.Get(bytes)
			req.Equal(result, want)
		}
	}

	type testCase struct {
		name  string
		bytes []byte
		want  uint32
	}

	testCases := []testCase{
		{
			name:  "Simple case",
			bytes: []byte("test"),
			want:  uint32(3632233996),
		},
		{
			name:  "Zero case",
			bytes: []byte(""),
			want:  uint32(0),
		},
	}

	// t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, hashCase(tc.bytes, tc.want))
	}
}
