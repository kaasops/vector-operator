package vectorpipeline

import (
	"encoding/json"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/utils"
)

func GetVpSpecHash(vp *vectorv1alpha1.VectorPipeline) (*uint32, error) {
	a, err := json.Marshal(vp.Spec)
	if err != nil {
		return nil, err
	}
	hash := utils.GetHash(a)
	return &hash, nil
}
