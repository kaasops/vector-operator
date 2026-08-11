package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kaasops/vector-operator/api/v1alpha1"
)

func statusReason(p Pipeline) *string {
	switch v := p.(type) {
	case *v1alpha1.VectorPipeline:
		return v.Status.Reason
	case *v1alpha1.ClusterVectorPipeline:
		return v.Status.Reason
	}
	return nil
}

// The reconcile sets the role on the pipeline before running configcheck, so the status
// write has to carry it even though the base was read before that, and it has to land on
// a pipeline someone else edited while configcheck ran. Both kinds go through the same
// reconcile, so both are covered.
func TestSetSuccessStatusStalePipeline(t *testing.T) {
	failedStatus := func() v1alpha1.VectorPipelineStatus {
		reason := "config check failed"
		failed := false
		return v1alpha1.VectorPipelineStatus{ConfigCheckResult: &failed, Reason: &reason}
	}

	type testCase struct {
		name  string
		seed  Pipeline
		empty func() Pipeline
	}

	testCases := []testCase{
		{
			name: "VectorPipeline",
			seed: &v1alpha1.VectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline", Namespace: "vector"},
				Status:     failedStatus(),
			},
			empty: func() Pipeline { return &v1alpha1.VectorPipeline{} },
		},
		{
			name: "ClusterVectorPipeline",
			seed: &v1alpha1.ClusterVectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-pipeline"},
				Status:     failedStatus(),
			},
			empty: func() Pipeline { return &v1alpha1.ClusterVectorPipeline{} },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := require.New(t)
			ctx := context.Background()

			s := runtime.NewScheme()
			req.NoError(clientgoscheme.AddToScheme(s))
			req.NoError(v1alpha1.AddToScheme(s))

			cl := crfake.NewClientBuilder().
				WithScheme(s).
				WithStatusSubresource(&v1alpha1.VectorPipeline{}, &v1alpha1.ClusterVectorPipeline{}).
				WithObjects(tc.seed).
				Build()
			key := client.ObjectKeyFromObject(tc.seed)

			stale := tc.empty()
			req.NoError(cl.Get(ctx, key, stale))
			base := stale.DeepCopyObject().(Pipeline)

			edited := tc.empty()
			req.NoError(cl.Get(ctx, key, edited))
			edited.SetAnnotations(map[string]string{"edited": "by-someone-else"})
			req.NoError(cl.Update(ctx, edited))

			role := v1alpha1.VectorPipelineRoleAgent
			stale.SetRole(&role)
			req.NoError(SetSuccessStatus(ctx, cl, stale, base))

			result := tc.empty()
			req.NoError(cl.Get(ctx, key, result))
			req.Equal(role, result.GetRole())
			req.True(result.IsValid())
			req.Nil(statusReason(result), "the previous failure reason must be cleared")
			req.NotNil(result.GetLastAppliedPipeline())
			req.Equal("by-someone-else", result.GetAnnotations()["edited"])
		})

		t.Run(tc.name+"/unobserved reason", func(t *testing.T) {
			req := require.New(t)
			ctx := context.Background()

			s := runtime.NewScheme()
			req.NoError(clientgoscheme.AddToScheme(s))
			req.NoError(v1alpha1.AddToScheme(s))

			seed := tc.seed.DeepCopyObject().(Pipeline)
			seed.SetReason(nil)
			seed.SetConfigCheck(false)
			cl := crfake.NewClientBuilder().
				WithScheme(s).
				WithStatusSubresource(&v1alpha1.VectorPipeline{}, &v1alpha1.ClusterVectorPipeline{}).
				WithObjects(seed).
				Build()
			key := client.ObjectKeyFromObject(seed)

			stale := tc.empty()
			req.NoError(cl.Get(ctx, key, stale))
			req.Nil(statusReason(stale), "the read must predate the failure")
			base := stale.DeepCopyObject().(Pipeline)

			failing := stale.DeepCopyObject().(Pipeline)
			req.NoError(SetFailedStatus(ctx, cl, failing, "config check failed", failing.DeepCopyObject().(Pipeline)))

			req.NoError(SetSuccessStatus(ctx, cl, stale, base))

			result := tc.empty()
			req.NoError(cl.Get(ctx, key, result))
			req.True(result.IsValid())
			req.Nil(statusReason(result), "a success must not keep a failure reason")
		})
	}
}
