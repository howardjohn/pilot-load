#!/usr/bin/env bash

set -eux

WD=$(dirname "$0")
WD=$(cd "$WD"; pwd)

kubectl create namespace istio-system || true
kubectl apply -f ${WD}/base-deployment.yaml

kubectl wait -n pilot-load --for=condition=available deployment/apiserver

kubectl port-forward -n pilot-load svc/apiserver 18090 &

sleep 1

if [[ "${MULTICLUSTER:-}" == "true" ]]; then
  cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: pilot-load-kubeconfig
  namespace: istio-system
  labels:
    istio/multiCluster: "true"
type: Opaque
data:
  config: "YXBpVmVyc2lvbjogdjEKY2x1c3RlcnM6Ci0gY2x1c3RlcjoKICAgIHNlcnZlcjogaHR0cDovL2FwaXNlcnZlci5waWxvdC1sb2FkOjE4MDkwCiAgbmFtZTogbG9hZApjb250ZXh0czoKLSBjb250ZXh0OgogICAgY2x1c3RlcjogbG9hZAogICAgdXNlcjogZmFrZQogIG5hbWU6IGxvYWQKY3VycmVudC1jb250ZXh0OiBsb2FkCmtpbmQ6IENvbmZpZwpwcmVmZXJlbmNlczoge30KdXNlcnM6Ci0gbmFtZTogZmFrZQo="
EOF
else
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: istio-kubeconfig
  namespace: istio-system
type: Opaque
data:
  config: YXBpVmVyc2lvbjogdjEKY2x1c3RlcnM6Ci0gY2x1c3RlcjoKICAgIHNlcnZlcjogaHR0cDovL2FwaXNlcnZlci5waWxvdC1sb2FkOjE4MDkwCiAgbmFtZTogbG9hZApjb250ZXh0czoKLSBjb250ZXh0OgogICAgY2x1c3RlcjogbG9hZAogICAgdXNlcjogZmFrZQogIG5hbWU6IGxvYWQKY3VycmVudC1jb250ZXh0OiBsb2FkCmtpbmQ6IENvbmZpZwpwcmVmZXJlbmNlczoge30KdXNlcnM6Ci0gbmFtZTogZmFrZQo=
EOF
fi
kubectl rollout restart deployment -n istio-system istiod || true

if [[ "${SINGLE:-}" != "false" ]]; then
  kubectl delete hpa istiod -n istio-system || true
  kubectl scale deployment/istiod --replicas=1 -n istio-system || true
fi

export KUBECONFIG=${WD}/local-kubeconfig.yaml
kubectl create namespace istio-system || true
kubectl apply -f $GOPATH/istio/manifests/charts/base/crds/
kubectl apply -f $WD/preconfigured.yaml

echo To start test: go run main.go
echo "export KUBECONFIG=${WD}/local-kubeconfig.yaml"