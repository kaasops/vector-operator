<p align="center">
  <a href="https://goreportcard.com/report/github.com/kaasops/vector-operator">
    <img src="https://goreportcard.com/badge/github.com/kaasops/vector-operator" alt="Go Report Card">
  </a>
</p>


## Description
The operator deploys and configures a vector agent daemonset on every node to collect container and application logs from the node file system.

## Features

- [x] Building vector config from namespaced custom resources (kind: VectorPipeline)
- [x] Configuration validation
- [x] Full support of vector config options
- [ ] Namespace isolation
- [ ] Garbage collection
- [ ] Vector config optimization
- [ ] Vector aggregator support


## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster
1. Install Instances of Custom Resources:

```sh
kubectl apply -f config/samples/
```

2. Build and push your image to the location specified by `IMG`:
	
```sh
make docker-build docker-push IMG=docker pull kaasops/vector-operator:latest
```
	
3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=docker pull kaasops/vector-operator:latest
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller to the cluster:

```sh
make undeploy
```

## Configuration Examples 
Configuration for CR Vector:
```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: Vector
metadata:
  name: vector-sample
  namespace: vector
spec:
  agent:
    service: true
    image: "timberio/vector:0.24.0-distroless-libc"
```

Configuration for CR VectorPipeline:
```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: VectorPipeline
metadata:
  name: vectorpipeline-sample
spec:
  sources:
    source1:
      type: "kubernetes_logs"
      extra_label_selector: "app!=testdeployment"
    source2:
      type: "kubernetes_logs"
      extra_label_selector: "app!=testdeployment1"
  transforms:
    remap:
      type: "remap"
      inputs:
        - source1
      source: |
        .@timestamp = del(.timestamp)

        .testField = "testValuevalue"
    filter:
      type: "filter"
      inputs:
        - source2
      condition:
        type: "vrl"
        source: ".status != 200"
  sinks:
    test222:
      type: "console"
      encoding:
        codec: "json"
      inputs:
        - filter
        - remap
```




## Contributing

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) 
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster 

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

