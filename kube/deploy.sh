#!/usr/bin/env bash
set -eux

kubectl apply -f kube/deploy.yaml

kubectl wait -n pilot-load --for=condition=available deployment/apiserver

kubectl port-forward -n pilot-load svc/apiserver 18090 &

sleep 1

KUBECONFIG=kube/local-kubeconfig.yaml kubectl create namespace istio-system || true
KUBECONFIG=kube/local-kubeconfig.yaml kubectl apply -f $GOPATH/src/istio.io/istio/manifests/base/files/crd-all.gen.yaml

kill 0

echo To start test: go run main.go