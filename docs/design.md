# Design
This document describes the design and interaction between the custom resource definitions (CRD) that the Vector Operator introduces.

Operator introduces the following custom resources:
- [Vector](#Vector)
- [VectorPipeline](#vectorpipeline)
- [ClusterVectorPipeline](#clustervectorpipeline)

# Vector
The `Vector` CRD declaratively defines a Vector installation to run in a Kubernetes cluster.
For each Vector resource, the Operator deploys a properly configured DaemonSet in the same namespace.
For each Vector resource, the Operator adds:
- DaemonSet with Vector
- Secret with Vector Configurtion file
- Service. (For connect to Vector API, or scrape Vector metrics, if enabled)
- ServiceAccount/ClusterRole/RoleBinding for get access to Kubernetes API


## Restrictions
- Currently tested only ONE installation Vector on Kubernetes cluster

## Planned
- Add aggregator role in StatefullSet
- Add features for compress Vector configuration file. (Delete dublicates sources/Transforms/Sinks. Compress to gzip)

## Specification
Specification access to [this]() page


# VectorPipeline
The `VectorPipeline` CRD defines Sources, Transforms and Sinks rules for Vector.
All `VectorPipelines`, with validated configuration file, added to Vector configuration file.

## Restrictions
- For source available only [kubernetes_logs](https://vector.dev/docs/reference/configuration/sources/kubernetes_logs/) type
- For source field `extra_namespace_label_selector` cannot be installed. The operator control this field and sets the namespace there, where VectorPipeline is defined.

## Specification
Specification access to [this]() page

# ClusterVectorPipeline
The `ClusterVectorPipeline` CRD defines Sources, Transforms and Sinks rules for Vector.
All `ClusterVectorPipelines`, with validated configuration file, added to Vector configuration file.

ClusterVectorPipelines works like VectorPipeline, but without restrictions.

## Specification
Specification access to [this]() page