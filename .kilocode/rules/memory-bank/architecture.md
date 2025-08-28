# Vector Operator Architecture

## System Architecture
The Vector Operator follows Kubernetes operator patterns to manage Vector deployments:

1. **Custom Resource Definitions (CRDs)**:
   - `VectorPipeline` and `ClusterVectorPipeline` - Define Vector data pipelines
   - `VectorAggregator` and `ClusterVectorAggregator` - Define Vector aggregator instances

2. **Controllers**:
   - `ClusterVectorAggregatorReconciler` - Manages cluster-wide Vector aggregators
   - `PipelineReconciler` - Manages Vector pipelines and their integration with agents/aggregators

3. **Configuration Building**:
   - `BuildAggregatorConfig` and `BuildAggregatorConfigVP` - Construct Vector configurations from pipeline specs
   - Configuration includes sources, transforms, and sinks
   - Handles Kubernetes events and metrics collection

4. **Resource Management**:
   - Manages Kubernetes resources like Deployments, Services, Secrets, ConfigMaps
   - Supports Prometheus PodMonitors for monitoring
   - Handles RBAC resources for proper permissions

## Source Code Paths
- `internal/config/` - Contains configuration building logic
- `internal/controller/` - Contains Kubernetes controller implementations
- `internal/pipeline/` - Pipeline management and filtering logic
- `internal/vector/` - Vector-specific agent and aggregator controllers
- `api/v1alpha1/` - Custom Resource Definitions and API types

## Key Technical Decisions
- Use of controller-runtime for Kubernetes operator patterns
- Separation of agent and aggregator configuration logic
- Pipeline-based configuration approach
- Integration with Prometheus operator for monitoring
- Event-driven architecture for reactive updates

## Component Relationships
- VectorPipeline → VectorConfigParams → VectorConfig
- ClusterVectorPipeline → VectorConfigParams → VectorConfig
- VectorAggregator → VectorConfig → Kubernetes Resources
- ClusterVectorAggregator → VectorConfig → Kubernetes Resources

## Critical Implementation Paths
- Pipeline reconciliation triggers aggregator updates
- Configuration validation before resource creation
- Event channels for cross-controller communication
- Service port management for Kubernetes services
