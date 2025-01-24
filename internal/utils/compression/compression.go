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

func Decompress(data []byte, log logr.Logger) []byte {
	if len(data) == 0 {
		return []byte{}
	}

	reader := bytes.NewReader(data)
	gz, err := gzip.NewReader(reader)
	if err != nil {
		log.Error(err, "Failed to create gzip reader for decompress")
		return nil
	}
	defer func() {
		if err := gz.Close(); err != nil {
			log.Error(err, "Failed to close reader for decompress")
		}
	}()

	var result bytes.Buffer
	if _, err := result.ReadFrom(gz); err != nil {
		log.Error(err, "Failed to read decompressed data")
		return nil
	}
	return result.Bytes()
}
