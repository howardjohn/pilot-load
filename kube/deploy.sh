#!/usr/bin/env bash
set -eux

kubectl apply -f kube/deploy.yaml

kubectl wait -n pilot-load --for=condition=available deployment/apiserver

kubectl port-forward -n pilot-load svc/apiserver 18090 &

sleep 1

export KUBECONFIG=kube/local-kubeconfig.yaml
kubectl create namespace istio-system || true
kubectl apply -f $GOPATH/src/istio.io/istio/manifests/charts/base/crds/

echo To start test: go run main.go
echo 'export KUBECONFIG=kube/local-kubeconfig.yaml'