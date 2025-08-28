# Current Project Context

## Core Functionality
The Vector Operator currently manages Vector deployments in Kubernetes by:
- Watching VectorPipeline and ClusterVectorPipeline custom resources
- Building Vector configurations from these pipeline manifests
- Supporting both agent and aggregator roles
- Validating configurations before applying them
- Managing Kubernetes resources (Deployments, Services, Secrets, ConfigMaps)

## Current Enhancement Focus
The project is focused on enhancing the VectorAggregator functionality to properly construct configurations from VectorPipeline manifests. This involves:
- Improving the `BuildAggregatorConfig` function to handle pipeline configurations
- Ensuring proper integration between VectorPipeline and VectorAggregator resources
- Maintaining backward compatibility with existing configurations

## Key Files
- `internal/config/aggregator.go` - Contains configuration building logic
- `internal/controller/clustervectoraggregator_controller.go` - ClusterVectorAggregator controller
- `internal/controller/pipeline_controller.go` - Pipeline controller
- `api/v1alpha1/vectorpipeline.go` - VectorPipeline API definitions

## Recent Changes
- Initial implementation of VectorPipeline and ClusterVectorPipeline controllers
- Basic configuration building for aggregators
- Integration with Prometheus operator for monitoring

## Next Steps
- Complete the VectorAggregator configuration building from VectorPipeline manifests
- Add comprehensive testing for the new functionality
- Update documentation to reflect the new capabilities

## Hints
- if you need kubernetes fake client for tests, use `k8s.io/client-go/kubernetes`
