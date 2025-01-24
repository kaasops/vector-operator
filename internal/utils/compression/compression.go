package compression

import (
	"bytes"
	"compress/gzip"

	"github.com/go-logr/logr"
)

func Compress(data []byte, log logr.Logger) []byte {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(data); err != nil {
		log.Error(err, "Failed to compress")
	}

	if err := gz.Close(); err != nil {
		log.Error(err, "Failed to close writer for compress")
	}

	return b.Bytes()
}
