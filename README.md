# Pilot Load Testing

This repo contains tooling to perform low cost load testing of Pilot.

## Architecture

A standalone etcd and api-server will be deployed to namespace `pilot-load`. These will be used just as an object store
essentially - there is no kubelet, controller-manager, kube-proxy, etc. No pods are scheduled onto any physical machine, and
exist only as objects in the data base.

Another deployment, `pilot-load` is also deployed, and will connect to our fake api-server. Based on the config provided,
Pods, Services, Endpoints, and Istio configuration will be applied to the api-server. Because there is no controller-manager, we need
to manage some objects ourselves, such as keeping Endpoints in sync with pods.

Pilot will be modified to use a KUBECONFIG pointing to our fake api-server. As a result, it will get events from the api-server
just like it would in a real cluster, but without the cost. This exercises the exact same code paths as a real cluster; it is not using
any "fake" client.

In a real cluster, each pod adds additional load as well. These are also handled. When a pod is created, we will directly send
an injection request and CSR, simulating what a real pod would do. We will then start an XDS connection to Pilot, simulating the
same pattern that a real Envoy would.

Overall, we get something that is very close to the load on a real cluster, with much smaller overhead. etcd and api-server generally use
less than 1 CPU/1 GB combined, and pilot-load will use roughly 1 CPU/1k pods. Compared to 1 CPU/4 pods, we get a ~250x efficiency gain. Additionally,
because we run our own api-server without rate limits, we can perform operations much faster.

The expense of this is dropping coverage:
* No coverage of any data plane aspects
* Less coverage of overloading Kubernetes
* The simulated behavior may not exactly match reality

## Getting Started

1. Install Istio in cluster. No special configuration is needed. You may want to ensure no Envoy's are connected, as they will be sent invalid configuration

1. Run [`./install/deploy.sh`](./install/deploy.sh). This will configure the api-server and kubeconfig to access it. It will also bootstrap the cluster with CRDs and telemetry filters.

1. Restart istiod to pick up the new kubeconfig: `kubectl rollout restart deployment -n istio-system istiod`.

1. Deploy the load test

    1. In cluster:

      ```shell script
      # Select a configuration to run
      kubectl apply -f install/configs/canonical.yaml
      # Apply the actual deployment
      kubectl apply -f install/load-deployment.yaml
      ```

    1. Locally:

      ```shell script
      # Connect to the remote kubeconfig
      kubectl port-forward -n pilot-load svc/apiserver 18090
      export KUBECONFIG=kube/local-kubeconfig.yaml
      # Connect to Istiod, if its not running locally as well
      kubectl port-forward -n istio-system svc/istiod 15010
      # Apply the actual deployment
      pilot-load cluster --config example-config.yaml
      ```
1. Optional: Import the [load testing dashboard](./install/dashboard.json) in Grafana.

## XDS Only

To just simulate XDS connections, without any api-server interaction, the adsc mode can be used:

```shell script
pilot-load adsc --adsc.count=2
```

This will start up two XDS connections.

NOTE: these connections will not be associated with any Services, and as such will get a different config than real pods, including sidecar scoping.
