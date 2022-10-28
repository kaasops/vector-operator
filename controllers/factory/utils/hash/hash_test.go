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
