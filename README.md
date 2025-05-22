# Pilot Load Testing

This repo contains tooling to perform low cost load testing of Pilot.

## Configuration Simulator

The primary tool in this repo is used to simulate clusters with large scale but low costs.
This includes applying configuration resources (builtin and CRDs) as well as Pods, Endpoints, Services, etc.

Importantly, despite creating Pods, **no pods are scheduled onto any physical machine**.
Instead, the tool acts as a fake kubelet, registering fake `Nodes` into the cluster and assigning `Pods` to these nodes.
This gives the appearance of a real environment -- a controller will see nothing different -- but without the costs.

> [!NOTE]
> Formerly, this repo stood up a standalone etcd and api-server that controllers could connect to.
> This method is generally not useful unless you are running in an environment where the api-server is slow (cloud providers)
> or you want multi-cluster.

In order to apply large scale configurations, the tool takes in a declarative spec of the desired cluster state.
This includes creating many namespaces (which can have copies), which can have arbitrary config applied to them (via templates).
This allows, for example, to easily create 100 namespaces, each with 10 services, each with an HTTPRoute and 10 pods.

Additionally, when pods are created as Istio pods (sidecar, gateway, waypoint, etc), XDS connections will also be opened for each,
impersonating the pod.

### Usage

```shell
pilot-load cluster --config examples/basic.yaml
```

See other examples for more complex usage.

Under [`install`](./install) there are some examples of running this in-cluster.
However, I never use this so its likely out of date and broken.

Optional: Import the [load testing dashboard](./install/dashboard.json) in Grafana.

## XDS Only

To just simulate XDS connections, without any api-server interaction, the adsc mode can be used:

```shell script
pilot-load adsc --count=2
```

This will start up two XDS connections.

NOTE: these connections will not be associated with any Services, and as such will get a different config than real pods, including sidecar scoping.

## Reproduce

The `reproduce-cluster` command allows replaying a cluster's configuration. Install `kubectl grep`
[plugin](https://github.com/howardjohn/kubectl-grep) before running below command.

First, capture their current cluster config: 

`kubectl get authorizationpolicies,destinationrules,envoyfilters,gateways,peerauthentications,requestauthentications,serviceentries,sidecars,telemetries,virtualservices,workloadgroups,workloadentries,configmaps,svc,endpoints,pod,namespace -oyaml -A | kubectl grep > my-config.yaml`

Then:

1. Locally

```shell script
pilot-load reproduce-cluster -f my-config.yaml --delay=50ms
```

This will deploy all the configs to the cluster, except Pods. For each pod, an XDS connection simulating that pod will be made.
Some resources are slightly modified to allow running in a cluster they were not originally in, such as Service selectors.

## Pod startup speed

The `pod-startup` command tests pod startup times

Example usage:

```shell script
pilot-load pod-startup --namespace default --concurrency 2
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

| Metric     | Meaning                                                                                                                                                            |
|------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| scheduled  | Time from start until init container starts (TODO: this is always 0 without sideacr)                                                                               |
| init       | Time from `scheduled` until the application container starts                                                                                                       |
| ready      | Time from application container starting until kubelet reports readiness                                                                                           |
| full ready | Time from application container starting until the Pod spec is fully declared as "Ready". This may be high than `ready` due to latency in kubelet updating the Pod |
| complete   | End to end time to completion                                                                                                                                      |

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
