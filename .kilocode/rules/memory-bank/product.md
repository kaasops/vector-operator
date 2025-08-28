# Vector Operator Product Overview

## Purpose
The Vector Operator is a Kubernetes operator that manages the configuration and deployment of Vector (an observability data pipeline) within Kubernetes clusters. It automates the lifecycle management of Vector instances, including both agents and aggregators, by watching custom resource definitions and synchronizing the cluster state with the desired Vector configuration.

## Key Features
- **Vector Pipeline Management**: Watches VectorPipeline and ClusterVectorPipeline custom resources
- **Configuration Building**: Constructs Vector configurations from Kubernetes manifests
- **Multi-role Support**: Handles both Vector agents and aggregators
- **Validation**: Validates configurations before applying them
- **Resource Management**: Manages Kubernetes resources like Deployments, Services, Secrets, ConfigMaps
- **Monitoring Integration**: Supports Prometheus operator for metrics collection
- **Event Handling**: Processes Kubernetes events and logs for observability

## User Experience Goals
- Provide declarative configuration of Vector pipelines and aggregators
- Automate the deployment and updates of Vector instances
- Ensure configuration validation and proper error handling
- Integrate seamlessly with Kubernetes ecosystem tools
- Support both namespace-scoped and cluster-scoped Vector deployments
