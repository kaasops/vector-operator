package vectorpipeline

import (
	"encoding/json"
	"hash/crc32"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func GetHash(input []byte) uint32 {
	crc32q := crc32.MakeTable(crc32.IEEE)
	return crc32.Checksum(input, crc32q)
}

func GetVpSpecHash(vp *vectorv1alpha1.VectorPipeline) (*uint32, error) {
	a, err := json.Marshal(vp.Spec)
	if err != nil {
		return nil, err
	}
	hash := GetHash(a)
	return &hash, nil
}
