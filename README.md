# Pilot Load

## Install

1. Install Istio in cluster

1. Run `./kube/deploy.sh`. This will configure a new apiserver and mutlicluster secrets to access this.

1. To deploy in cluster, run `kubectl apply -f install/deployment`.

1. To deploy out of cluster, run

```shell script
export KUBECONFIG=kube/local-kubeconfig.yaml
kubectl port-forward -n istio-system svc/istiod 15010
pilot-load <simulation>
```
