#!/usr/bin/env bash
set -eux

kubectl apply -f kube/deploy.yaml

kubectl wait -n pilot-load --for=condition=available deployment/apiserver

kubectl port-forward -n pilot-load svc/apiserver 18090 &
kubectl port-forward -n istio-system svc/istio-pilot 15010 &

sleep 5

cat <<EOF | kubectl apply -n istio-system -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: pilot-load-kubeconfig
  namespace: istio-system
data:
  kubeconfig.yaml: |
    apiVersion: v1
    clusters:
    - cluster:
        server: http://apiserver.pilot-load:18090
      name: load
    contexts:
    - context:
        cluster: load
        namespace: workload
        user: ""
      name: load
    current-context: load
    kind: Config
    preferences: {}
    users: []
EOF
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
                            "--secureGrpcAddr",
                            "",
                            "--kubeconfig=/etc/istio/kubeconfig/kubeconfig.yaml"
                        ],
                        "env": [
                            {
                                "name": "PILOT_CERT_PROVIDER",
                                "value": "citadel"
                            }
                        ],
                        "volumeMounts": [
                            {
                                "mountPath": "/etc/istio/kubeconfig",
                                "name": "kubeconfig"
                            },
                            {
                                "mountPath": "/var/lib/istio/inject",
                                "name": "kubeconfig"
                            },
                            {
                                "mountPath": "/var/lib/istio/validation",
                                "name": "kubeconfig"
                            }
                        ],
                        "name": "discovery"
                    }
                ],
                "volumes": [
                    {
                        "configMap": {
                            "defaultMode": 420,
                            "name": "pilot-load-kubeconfig"
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

export KUBECONFIG=kube/kubeconfig.yaml
kubectl create namespace istio-system || true
kubectl apply -f $GOPATH/src/istio.io/istio/manifests/base/files/crd-all.gen.yaml

echo To start test: go run main.go