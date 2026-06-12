package common

const (
	AnnotationServiceName = "observability.kaasops.io/service-name"
	AnnotationRestartedAt = "vector-operator.kaasops.io/restartedAt"
	// AnnotationConfigOptimization set to "disabled" on a Vector CR opts the agent
	// out of the config optimization enabled by --enable-config-optimization.
	AnnotationConfigOptimization = "vector-operator.kaasops.io/config-optimization"
)
