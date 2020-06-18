#!/usr/bin/env bash
set -eux

kubectl apply -f kube/deploy.yaml

kubectl wait -n pilot-load --for=condition=available deployment/apiserver

kubectl port-forward -n pilot-load svc/apiserver 18090 &

sleep 1

if [[ "${PATCH:-}" == "true" ]]; then
  cat <<EOF > /tmp/patch.json
{
    "spec": {
        "template": {
            "spec": {
                "containers": [
                    {
                        "args": [
                            "discovery",
                            "--monitoringAddr=:15014",
                            "--keepaliveMaxServerConnectionAge=30m",
                            "--kubeconfig=/etc/istio/kubeconfig/pilot-load"
                        ],
                        "env": [{"name":"INJECTION_WEBHOOK_CONFIG_NAME","value":""}],
                        "volumeMounts": [
                            {
                                "mountPath": "/etc/istio/kubeconfig",
                                "name": "kubeconfig"
                            }
                        ],
                        "name": "discovery"
                    }
                ],
                "volumes": [
                    {
                        "secret": {
                            "defaultMode": 420,
                            "secretName": "pilot-load-multicluster"
                        },
                        "name": "kubeconfig"
                    }
                ]
            }
        }
    }
}
EOF
  kubectl patch deployment -n istio-system istiod --patch "$(cat /tmp/patch.json)"
  kubectl label secret -n istio-system pilot-load-multicluster istio/multiCluster-
fi

if [[ "${SINGLE:-}" != "true" ]]; then
  kubectl delete hpa istiod -n istio-system || true
  kubectl scale deployment/istiod --replicas=1 -n istio-system
fi


export KUBECONFIG=kube/local-kubeconfig.yaml
kubectl create namespace istio-system || true
kubectl apply -f $GOPATH/src/istio.io/istio/manifests/charts/base/crds/


echo To start test: go run main.go
echo 'export KUBECONFIG=kube/local-kubeconfig.yaml'