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

package hash

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	hashCase := func(bytes []byte, want uint32) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			result := Get(bytes)
			req.Equal(result, want)
		}
	}

	t.Run("Simple case", hashCase([]byte("test"), uint32(3632233996)))
	t.Run("Zero case", hashCase([]byte(""), uint32(0)))
}
