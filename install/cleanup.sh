#!/usr/bin/env bash

set -x

kubectl delete pods,svc,ep,vs,dr,nodes -A --all --force --grace-period=0 --wait=false
namespaces=$(kubectl get namespace -oname | cut - -d/ -f2 | egrep -v '(istio-system|kube-|default )')
for ns in $namespaces; do
  kubectl delete ns $ns --wait=false
  echo '{"kind":"Namespace","spec":{"finalizers":[]},"apiVersion":"v1","metadata":{"name":"'$ns'"}}' | kubectl replace --raw /api/v1/namespaces/$ns/finalize -f -
done
