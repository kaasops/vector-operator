package common

const (
	AnnotationServiceName = "observability.kaasops.io/service-name"
	AnnotationRestartedAt = "vector-operator.kaasops.io/restartedAt"
	// AnnotationConfigOptimization set to AnnotationValueDisabled opts out of the
	// config optimization enabled by --enable-config-optimization. On a Vector CR it
	// opts the whole agent out; on a (Cluster)VectorPipeline it keeps just that
	// pipeline's kubernetes_logs source standalone while the rest of the group still
	// collapses.
	AnnotationConfigOptimization = "vector-operator.kaasops.io/config-optimization"

	// AnnotationValueDisabled is the opt-out value for AnnotationConfigOptimization.
	AnnotationValueDisabled = "disabled"
)
