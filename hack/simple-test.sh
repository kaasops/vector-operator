#!/usr/bin/env bash

# This script test Vector Operator with simple settings

NAMESPACE=test-vector-operator
TESTAPP=log-spamer

function error_action() {
    cleanup_action
    exit 1
}

function cleanup_action() {
    kubectl delete ns ${NAMESPACE}
}

function check_command() {
    local command=$1

    if ! command -v $command &> /dev/null; then
        echo "Error: ${command} not found"
        exit 1
    fi
}

check_command kubectl
check_command helm

# Install Vector Operator
echo `date`": INFO: Add vector-operator helm repository"
error_add_helm_repo=$(helm repo add vector-operator https://kaasops.github.io/vector-operator/helm 2>&1)
if [ $? -ne 0 ]; then
    echo `date`": $error_add_helm_repo"
    exit 1
fi

echo `date`": INFO: Update helm repositories"
error_update_helm_repo=$(helm repo update 2>&1)
if [ $? -ne 0 ]; then
    echo `date`": $error_update_helm_repo"
    exit 1
fi

echo `date`": INFO: Create Namespace ${NAMESPACE} for Vector Operator"
error_create_ns=$(kubectl create ns ${NAMESPACE} 2>&1)
if [ $? -ne 0 ]; then
    echo `date`": $error_create_ns"
    exit 1
fi

echo `date`": INFO: Install Vector Operator"
error_install_vector_operator=$(helm install vector-operator vector-operator/vector-operator -n ${NAMESPACE} 2>&1)
if [ $? -ne 0 ]; then
    echo `date`": $error_install_vector_operator"
    error_action
fi

echo `date`": INFO: Deploy test application with logs"
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: ${TESTAPP}
  name: ${TESTAPP}
  namespace: ${NAMESPACE}
spec:
  replicas: 2
  selector:
    matchLabels:
      app: ${TESTAPP}
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      labels:
        app: ${TESTAPP}
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
        name: ${TESTAPP}
        resources:
          limits:
            cpu: 100m
            memory: 100Mi
          requests:
            cpu: 10m
            memory: 80Mi
EOF

echo `date`": INFO: Deploy Vector CR"
cat <<EOF | kubectl apply -f -
apiVersion: observability.kaasops.io/v1alpha1
kind: Vector
metadata:
  name: sample
  namespace: ${NAMESPACE}
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

echo `date`": INFO: Deploy VectorPipeLine CR"
cat <<EOF | kubectl apply -f -
apiVersion: observability.kaasops.io/v1alpha1
kind: VectorPipeline
metadata:
  name: sample
  namespace: ${NAMESPACE}
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

echo `date`": INFO: Wait 1m second"
sleep 60


echo `date`": INFO: Check logs"
kubectl logs -n ${NAMESPACE} -l app.kubernetes.io/component=Agent,app.kubernetes.io/instance=sample --tail 100 | grep ${NAMESPACE}_${TESTAPP} | grep "message"
if [ $? -ne 0 ]; then
    echo `date`": ERROR: Don't see logs in Vector Agent pods& Something work incorrectly"
    error_action
fi

echo `date`": INFO: All good. Logs Exist in Vector Agent Pods!"

cleanup_action
