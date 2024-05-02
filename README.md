 Pilot Load Testing

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

2. Install `pilot-load` by running `go install`.

3. Run [`./install/deploy.sh`](./install/deploy.sh). This will configure the api-server to access it. It will also bootstrap the cluster with CRDs and telemetry filters.

4. [Optional] For running pilot-load tool locally please enable port forwarding and set fake API server KUBECONFIG.  
```shell script
kubectl port-forward -n pilot-load svc/apiserver 18090 &
# Connect to Istiod, if its not running locally as well
kubectl port-forward -n istio-system svc/istiod 15010 &
export KUBECONFIG=install/local-kubeconfig.yaml
```

Below functionalities can be run via pilot-load tool.
## Cluster
7. Deploy the load test

    1. In cluster:

      ```shell script
      kubectl create namespace pilot-load
      # Select a configuration to run
      kubectl create configmap pilot-load-config -n pilot-load --from-file=config.yaml=examples/ambient.yaml --dry-run=client -oyaml | kubectl apply -f -
      # Apply the actual deployment
      kubectl apply -f install/load-deployment.yaml
      ```

    1. Locally:

      ```shell script
      pilot-load cluster --config examples/basic.yaml
      ```
8. Optional: Import the [load testing dashboard](./install/dashboard.json) in Grafana.

## Discovery Address

All commands take a few flags related to connecting to Istiod. Some common examples:

```shell
pilot-load adsc --pilot-address foo.com:80 --auth plaintext # no auth at all
pilot-load adsc --pilot-address meshconfig.googleapis.com # infers google auth. Cluster information is inferred but can be set explicitly
```

## XDS Only

To just simulate XDS connections, without any api-server interaction, the adsc mode can be used:

```shell script
pilot-load adsc --count=2
```

This will start up two XDS connections.

NOTE: these connections will not be associated with any Services, and as such will get a different config than real pods, including sidecar scoping.

## Ingress Prober

Note: this is independent of the above fake api server and can be run on a real cluster.

This test continuously applies virtual services and sends traffic to see how long it takes for a virtual service to become ready.

Usage: `pilot-load prober --replicas=1000 --delay=1s`.

## Reproduce
The `reproduce` command allows replaying a cluster's configuration. Install `kubectl grep`
[plugin](https://github.com/howardjohn/kubectl-grep)  before running below command.

First, capture their current cluster config: 

`kubectl get authorizationpolicies,destinationrules,envoyfilters,gateways,peerauthentications,requestauthentications,serviceentries,sidecars,telemetries,virtualservices,workloadgroups,workloadentries,configmaps,svc,endpoints,pod,namespace -oyaml -A | kubectl grep > my-config.yaml`

Then:

1. Locally

```shell script
pilot-load reproduce -f my-config.yaml --delay=50ms
```

This will deploy all the configs to the cluster, except Pods. For each pod, an XDS connection simulating that pod will be made.
Some resources are slightly modified to allow running in a cluster they were not originally in, such as Service selectors.

2. In cluster
Creation of config map with reproduce configs might fail as reproduce configs will be big. For this we can deploy a container with pilot-load
binary in it and use kubectl cp to copy reproduce config from local machine to container.
   1. Apply deployment
   ```shell script
      # Apply the actual deployment
      kubectl apply -f install/load-deployment-reproduce.yaml
   ```
   2. copy reproduce config from local machine to load testing container
   ```shell script
       kubectl cp -n pilot-load my-config.yaml <pod name>:/etc/config/my-config.yaml -c pilot-load
   ```
   3. Run reproduce by accessing bash of container
   ```shell script
        kubectl exec -n pilot-load -it <pod name> -c pilot-load -- bash
        # In the bash
        pilot-load reproduce --pilot-address=istiod.istio-system:15010 --file /etc/config/my-config.yaml --delay=50ms --qps=5000
   ```

## Pod startup speed

The `startup` command tests pod startup times

Example usage:

```shell script
pilot-load startup --namespace default --concurrency 2
```

This will spin up 2 workers which will continually spawn pods, measure the latency, and then terminate them.
That is, there will be roughly 2 pods at all times with the command above.

Latency is report as each pod completes, and summary when the process is terminated.

Pods spin up a simple alpine image and serve a readiness probe doing a TCP health check.

Note: if testing Istio/etc, ensure the namespace specified has sidecar injection enabled.

Example:
```
2022-05-06T16:51:17.486681Z     info    Report: scheduled:0s    init:12.647s    ready:13.647s   full ready:1.417s       complete:14.065s        name:startup-test-kytobohu
2022-05-06T16:51:18.507336Z     info    Report: scheduled:0s    init:14.419s    ready:15.419s   full ready:1.436s       complete:15.856s        name:startup-test-ukbwqdfl
2022-05-06T16:51:18.555901Z     info    Avg:    scheduled:0s    init:7.673032973s       ready:1s        full ready:1.730793412s complete:9.403826385s
2022-05-06T16:51:18.556263Z     info    Max:    scheduled:0s    init:14.419771526s      ready:1s        full ready:3.515997645s complete:15.856143083s
```

Metric meanings:

|Metric| Meaning                                                                                                                                                            |
|------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|scheduled| Time from start until init container starts (TODO: this is always 0 without sideacr)                                                                               |
|init| Time from `scheduled` until the application container starts                                                                                                       |
|ready| Time from application container starting until kubelet reports readiness                                                                                           |
|full ready| Time from application container starting until the Pod spec is fully declared as "Ready". This may be high than `ready` due to latency in kubelet updating the Pod |
|complete| End to end time to completion|

## Dump


The `dump` command impersonates a pod over XDS and dumps the resulting XDS config to files.
The XDS is modified to point to the local files rather than dynamic XDS configuration.

Example usage:

```shell script
pilot-load dump --pod my-pod --namespace test -p localhost:15012 --out /tmp/envoy
```

Port 15012 is required to fetch certificates.

One this is done, envoy can be run locally:

```shell script
envoy -c /tmp/envoy/config.yaml
```
