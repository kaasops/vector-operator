package compression

import (
	"bytes"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

type testCase struct {
	name string
	data []byte
}

var testCases = []testCase{
	{
		name: "case1",
		data: []byte("Hello, World"),
	},
	{
		name: "caseEmpty",
		data: []byte(""),
	},
	{
		name: "caseSpecialCharacters",
		data: []byte("!@#$%^&*()"),
	},
}

func TestCompressAndDecompress(t *testing.T) {
	var log logr.Logger
	zapLog, _ := zap.NewProduction()
	defer zapLog.Sync()
	log = zapr.NewLogger(zapLog)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			compressed := Compress(tc.data, log)
			decompressed := Decompress(compressed, log)

			if !bytes.Equal(tc.data, decompressed) {
				t.Errorf("Compression and Decompression failed: expected %s, got %s", tc.data, decompressed)
			}
		})
	}
}

func TestDecompressInvalidInput(t *testing.T) {
	var log logr.Logger
	zapLog, _ := zap.NewProduction()
	defer zapLog.Sync()
	log = zapr.NewLogger(zapLog)

	invalidCompressedData := []byte("wrong data")

	decompressed := Decompress(invalidCompressedData, log)

	if decompressed != nil {
		t.Errorf("Decompression should have failed and returned nil")
	}
}
