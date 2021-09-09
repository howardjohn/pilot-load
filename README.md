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

1. Install `pilot-load` by running `go install`.

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
      export KUBECONFIG=install/local-kubeconfig.yaml
      # Connect to Istiod, if its not running locally as well
      kubectl port-forward -n istio-system svc/istiod 15010
      # Apply the actual deployment
      pilot-load cluster --config example-config.yaml
      ```
1. Optional: Import the [load testing dashboard](./install/dashboard.json) in Grafana.

## Discovery Address

All commands take a few flags related to connecting to Istiod. Some common examples:

```shell
pilot-load adsc --pilot-address foo.com:80 --auth plaintext # no auth at all
pilot-load adsc --pilot-address meshconfig.googleapis.com # infers google auth. Cluster information is inferred but can be set explicitly
```

## XDS Only

To just simulate XDS connections, without any api-server interaction, the adsc mode can be used:

```shell script
pilot-load adsc --adsc.count=2
```

This will start up two XDS connections.

NOTE: these connections will not be associated with any Services, and as such will get a different config than real pods, including sidecar scoping.

## Ingress Prober

Note: this is independent of the above fake api server and can be run on a real cluster.

This test continuously applies virtual services and sends traffic to see how long it takes for a virtual service to become ready.

Usage: `pilot-load prober --prober.replicas=1000 --prober.delay=1s`.

## Reproduce

The `reproduce` command allows replaying a cluster's configuration.

First, capture their current cluster config: `kubectl get vs,gw,dr,sidecar,svc,endpoints,pod,namespace -oyaml -A | kubectl grep`

Then:

```shell script
pilot-load reproduce -f my-config.yaml --delay=50ms
```

This will deploy all of the configs to the cluster, except Pods. For each pod, an XDS connection simulating that pod will be made.
Some resources are slightly modified to allow running in a cluster they were not originally in, such as Service selectors.