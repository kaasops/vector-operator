# Vector Operator Technology Stack

## Core Technologies
- **Go Programming Language**: Primary implementation language
- **Kubernetes Operator Framework**: Uses controller-runtime for operator patterns
- **Custom Resource Definitions (CRDs)**: Defines VectorPipeline and VectorAggregator resources
- **Vector Configuration**: Uses Vector's JSON configuration format
- **Prometheus Operator**: For monitoring integration via PodMonitors

## Development Setup
- **Go Modules**: Dependency management via go.mod/go.sum
- **Kustomize**: For Kubernetes configuration management
- **Helm Charts**: For packaging and deployment
- **Controller-Runtime**: Kubernetes operator framework
- **Prometheus Operator**: For monitoring capabilities

## Key Dependencies
- `controller-runtime` - Kubernetes operator patterns
- `kubernetes` - Kubernetes client libraries
- `prometheus-operator` - Monitoring integration
- `go-strcase` - String case conversion utilities
- `errgroup` - Error handling for concurrent operations

## Technical Constraints
- Must maintain backward compatibility with existing Vector configurations
- Configuration validation required before resource creation
- Event-driven architecture for reactive updates
- Proper RBAC permissions management

## Tool Usage Patterns
- Uses `controller-runtime` for watching Kubernetes resources
- Uses `configcheck` for validating Vector configurations
- Uses `hash` for configuration change detection
- Uses `k8s` utilities for Kubernetes operations
- Uses `errgroup` for concurrent reconciliation of multiple Vector instances
