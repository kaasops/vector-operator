# Quick start
Operator serves to make running Vector applications on top of Kubernetes as easy as possible while preserving Kubernetes-native configuration options.

# Installing by Manifest
## Install CRDs
```bash
git clone https://github.com/kaasops/vector-operator.git
cd vector-operator
kubectl apply -f config/crd/bases/observability.kaasops.io_vectors.yaml
kubectl apply -f config/crd/bases/observability.kaasops.io_vectorpipelines.yaml
kubectl apply -f config/crd/bases/observability.kaasops.io_clustervectorpipelines.yaml      
```

## Start Vector Operator
### Create namespace for Vector Operator
```bash
kubectl create namespace vector
```

### Create RBAC
```bash
# Create Secret for Vector Operator Service Account (if Kubernetes version > 1.23)
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: vector-operator-sa-token
  namespace: vector
  annotations:
    kubernetes.io/service-account.name: vector-operator
type: kubernetes.io/service-account-token
EOF
```

```bash
# Create ServiceAccount for Vector Operator
kubectl create serviceaccount -n vector vector-operator

# Create ClusterRole for Vector Operator
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: vector-operator
rules:
- apiGroups:
  - ""
  resources:
  - secrets
  - pods
  - serviceaccounts
  - services
  - namespaces
  - nodes
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - rbac.authorization.k8s.io
  resources:
  - clusterroles
  - clusterrolebindings
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps
  resources:
  - daemonsets
  - replicasets
  - statefulsets
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps
  - extensions
  resources:
  - deployments
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - observability.kaasops.io
  resources:
  - clustervectorpipelines
  - vectorpipelines
  - vectors
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - observability.kaasops.io
  resources:
  - clustervectorpipelines/status
  - vectorpipelines/status
  - vectors/status
  verbs:
  - get
  - patch
  - update
EOF

# Create ClusterRoleBinding for Vector Operator
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: vector-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: vector-operator
subjects:
- kind: ServiceAccount
  name: vector-operator
  namespace: vector
EOF
```

### Create Vector Operator Deployment
```bash
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vector-operator
  namespace: vector
  labels:
    app.kubernetes.io/name: vector-operator
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: vector-operator
  template:
    metadata:
      labels:
        app.kubernetes.io/name: vector-operator
    spec:
      containers:
      - image: kaasops/vector-operator:latest
        imagePullPolicy: IfNotPresent
        name: vector-operator
        resources:
          limits:
            cpu: "1"
            memory: 1Gi
          requests:
            cpu: 100m
            memory: 50Mi
      serviceAccount: vector-operator
      serviceAccountName: vector-operator
EOF
```

## Deploy test application (log-spamer)
Deploy log-spamer application to cluster to test namespace
```bash
# Create Namespace for tests
kubectl create namespace test


cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: log-spamer
  name: log-spamer
  namespace: test
spec:
  replicas: 2
  selector:
    matchLabels:
      app: log-spamer
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      labels:
        app: log-spamer
    spec:
      containers:
      - env:
        - name: MESSAGE_PER_SECOND
          value: "100"
        - name: MESSAGE_SIZE_FROM
          value: "50"
        - name: MESSAGE_SIZE_TO
          value: "100"
        image: zvlb/log-spamer:latest
        imagePullPolicy: Always
        name: log-spamer
        resources:
          limits:
            cpu: 100m
            memory: 100Mi
          requests:
            cpu: 10m
            memory: 80Mi
EOF
```


## Deploy Vector
### Deploy minimal Vector CR
```bash
cat <<EOF | kubectl apply -f -
apiVersion: observability.kaasops.io/v1alpha1
kind: Vector
metadata:
  name: sample
  namespace: vector
spec:
  agent:
    tolerations:
    - effect: NoSchedule
      key: node-role.kubernetes.io/master
      operator: Exists
    - effect: NoSchedule
      key: node-role.kubernetes.io/control-plane
      operator: Exists
EOF
```

Check Vector DaemonSet pods:
```bash
kubectl get pod -n vector
```

### Deploy minimal VectorPipeline CR for get log-spamer logs
```bash
cat <<EOF | kubectl apply -f -
apiVersion: observability.kaasops.io/v1alpha1
kind: VectorPipeline
metadata:
  name: sample
  namespace: test
spec:
  sources:
    source-test:
      type: "kubernetes_logs"
      extra_label_selector: "app=log-spamer"
  transforms:
    transform-test:
      type: "remap"
      inputs:
        - source-test
      source: |
        .testfield = "test"
  sinks:
    sink-test:
      type: "console"
      encoding:
        codec: "json"
      inputs:
        - transform-test
EOF
```

Check VectorPipeline status:
```bash
kubectl get vp -n test
```
Output:
```bash
NAME     AGE   VALID
sample   48s   true
```

After some times you can see log-spamer logs in Vector stdout:
```bash
kubectl logs -n vector -l app.kubernetes.io/instance=sample -f
```
Output
```bash
{"file":"/var/log/pods/vector_log-spamer-788b9ffbf5-nmz4m_9585867b-5457-4729-a682-db3bed0ffd67/log-spamer/0.log","kubernetes":{"container_id":"containerd://d280076162fcd9a1521a8054c215521c1f2d7a4e8e72fe63b2195dd2b7d99b7d","container_image":"zvlb/log-spamer:latest","container_name":"log-spamer","namespace_labels":{"kubernetes.io/metadata.name":"vector"},"node_labels":{"beta.kubernetes.io/arch":"amd64","beta.kubernetes.io/os":"linux","kubernetes.io/arch":"amd64","kubernetes.io/hostname":"lux-kube-node13","kubernetes.io/os":"linux"},"pod_ip":"172.24.35.158","pod_ips":["172.24.35.158"],"pod_labels":{"app":"log-spamer","pod-template-hash":"788b9ffbf5"},"pod_name":"log-spamer-788b9ffbf5-nmz4m","pod_namespace":"vector","pod_node_name":"lux-kube-node13","pod_owner":"ReplicaSet/log-spamer-788b9ffbf5","pod_uid":"9585867b-5457-4729-a682-db3bed0ffd67"},"message":"{\"level\":\"info\",\"time\":\"2022-11-01T12:03:31Z\",\"message\":\"3irbVba4Sf8qFC2i78UfjVzwUGzBu3m3AnbMbSTXkkyqTcAaLtuL6S39hAVfqx\"}","source_type":"kubernetes_logs","stream":"stderr","testfield":"test","timestamp":"2022-11-01T12:03:31.766514243Z","timestamp_end":"2022-11-01T12:03:31.766514243Z"}
{"file":"/var/log/pods/vector_log-spamer-788b9ffbf5-nmz4m_9585867b-5457-4729-a682-db3bed0ffd67/log-spamer/0.log","kubernetes":{"container_id":"containerd://d280076162fcd9a1521a8054c215521c1f2d7a4e8e72fe63b2195dd2b7d99b7d","container_image":"zvlb/log-spamer:latest","container_name":"log-spamer","namespace_labels":{"kubernetes.io/metadata.name":"vector"},"node_labels":{"beta.kubernetes.io/arch":"amd64","beta.kubernetes.io/os":"linux","kubernetes.io/arch":"amd64","kubernetes.io/hostname":"lux-kube-node13","kubernetes.io/os":"linux"},"pod_ip":"172.24.35.158","pod_ips":["172.24.35.158"],"pod_labels":{"app":"log-spamer","pod-template-hash":"788b9ffbf5"},"pod_name":"log-spamer-788b9ffbf5-nmz4m","pod_namespace":"vector","pod_node_name":"lux-kube-node13","pod_owner":"ReplicaSet/log-spamer-788b9ffbf5","pod_uid":"9585867b-5457-4729-a682-db3bed0ffd67"},"message":"{\"level\":\"info\",\"time\":\"2022-11-01T12:03:31Z\",\"message\":\"SJtmGewiQcE9hnEtgCjxkzHZpWbvmTNB69temrBZ6pH3aMSGsa5WXFqPBRz7gVhmnYpmpQP7\"}","source_type":"kubernetes_logs","stream":"stderr","testfield":"test","timestamp":"2022-11-01T12:03:31.776769316Z","timestamp_end":"2022-11-01T12:03:31.776769316Z"}
```

You can see field `testfield`, which we add in transform section in VectorPipeline

# Cleanup
```bash
kubectl delete namespace test
kubectl delete namespace vector
kubectl delete clusterrole vector-operator
kubectl delete clusterrolebinding vector-operator
kubectl delete -f config/crd/bases/observability.kaasops.io_vectors.yaml
kubectl delete -f config/crd/bases/observability.kaasops.io_vectorpipelines.yaml
kubectl delete -f config/crd/bases/observability.kaasops.io_clustervectorpipelines.yaml
```