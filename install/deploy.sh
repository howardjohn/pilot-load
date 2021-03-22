#!/usr/bin/env bash

set -eux

WD=$(dirname "$0")
WD=$(cd "$WD"; pwd)

kubectl create namespace istio-system || true
kubectl apply -f ${WD}/base-deployment.yaml

kubectl wait -n pilot-load --for=condition=available deployment/etcd
kubectl wait -n pilot-load --for=condition=available deployment/apiserver

kubectl port-forward -n pilot-load svc/apiserver 18090 &

sleep 1

if [[ "${MULTICLUSTER:-}" != "true" ]]; then
  kubectl label secret -n istio-system istio-kubeconfig istio/multiCluster=true --overwrite=true
fi
kubectl rollout restart deployment -n istio-system istiod || true

if [[ "${SINGLE:-}" != "false" ]]; then
  kubectl delete hpa istiod -n istio-system || true
  kubectl scale deployment/istiod --replicas=1 -n istio-system || true
fi

export KUBECONFIG=${WD}/local-kubeconfig.yaml
kubectl create namespace istio-system || true
kubectl apply -f $GOPATH/src/istio.io/istio/manifests/charts/base/crds/
kubectl apply -f $WD/preconfigured.yaml

echo To start test: go run main.go
echo "export KUBECONFIG=${WD}/local-kubeconfig.yaml"