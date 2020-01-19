#!/usr/bin/env bash
set -eux

echo '{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"istio-system"}}' | kubectl create --raw '/api/v1/namespaces/pilot-load/services/apiserver:tcp/proxy/api/v1/namespaces' -f - || true

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
kubectl patch deployment -n istio-system istio-pilot --patch "$(cat /tmp/patch.json)"