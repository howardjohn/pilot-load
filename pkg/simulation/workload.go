package simulation

import (
	"context"
	"fmt"
	"log"

	"github.com/howardjohn/pilot-load/client"
)

var (
	namespaceYml = `
apiVersion: v1
kind: Namespace
metadata:
  labels:
    istio-injection: enabled
  name: {{.Name}}
spec:
status:
  phase: Active
`
)

type NamespaceSpec struct {
	Name string
}

func (s NamespaceSpec) Generate() string {
	return render(namespaceYml, s)
}

var (
	podYml = `
apiVersion: v1
kind: Pod
metadata:
  labels:
    app: {{.App}}
  name: {{.App}}-{{.UID}}
  namespace: {{.Namespace}}
  resourceVersion: "46749"
spec:
  containers:
  - image: alpine
    name: alpine
    ports:
    - containerPort: 80
      protocol: TCP
  - image: istio/proxyv2
    name: istio-proxy
    ports:
    - containerPort: 15090
      name: http-envoy-prom
      protocol: TCP
  initContainers:
  - image: istio/proxyv2
    imagePullPolicy: Always
    name: istio-init
  nodeName: {{.Node}}
  serviceAccountName: {{.ServiceAccount}}
status:
  phase: Running
  podIP: {{.IP}}
  podIPs:
  - ip: {{.IP}}
`
)

type PodSpec struct {
	ServiceAccount string
	Node           string
	App            string
	Namespace      string
	UID            string
	IP             string
}

func (s PodSpec) Generate() string {
	if s.UID == "" {
		s.UID = genUID()
	}
	return render(podYml, s)
}

var (
	serviceYml = `
apiVersion: v1
kind: Service
metadata:
  name: {{.App}}
  namespace:  {{.Namespace}}
spec:
  clusterIP: {{.IP}}
  ports:
  - name: http
    port: 80
    protocol: TCP
    targetPort: 80
  selector:
    app: {{.App}}
  type: ClusterIP
`
)

type ServiceSpec struct {
	App       string
	Namespace string
	IP        string
}

func (s ServiceSpec) Generate() string {
	return render(serviceYml, s)
}

var (
	endpointsYml = `
apiVersion: v1
kind: Endpoints
metadata:
  name: {{.App}}
  namespace: {{.Namespace}}
subsets:
- addresses:
{{- range $ip := .IPs }}
  - ip: {{$ip}}
    nodeName: {{$.Node}}
{{- end }}
  ports:
  - name: http
    port: 80
    protocol: TCP

`
)

type EndpointSpec struct {
	Node      string
	App       string
	Namespace string
	IPs       []string
}

func (s EndpointSpec) Generate() string {
	return render(endpointsYml, s)
}

var (
	serviceAccountYml = `
apiVersion: v1
kind: ServiceAccount
metadata:
  labels:
    app: {{.App}}
  name: {{.Name}}
  namespace: {{.Namespace}}
`
)

type ServiceAccountSpec struct {
	App       string
	Namespace string
	Name      string
}

func (s ServiceAccountSpec) Generate() string {
	return render(serviceAccountYml, s)
}

type Workload struct {
	App            string
	Node           string
	Namespace      string
	ServiceAccount string
	Instances      int
}

func (w Workload) Run(a Args) (func(context.Context) error, error) {
	return func(ctx context.Context) error {
		config, ips := createWorkload(w)
		log.Println("Applying config: ", config)
		if err := applyConfig(config); err != nil {
			return fmt.Errorf("failed to apply config: %v", err)
		}
		meta := map[string]interface{}{
			"ISTIO_VERSION": "1.5.0",
			"CLUSTER_ID":    "Kubernetes",
			"LABELS": map[string]string{
				"app": w.App,
			},
			"CONFIG_NAMESPACE": w.Namespace,
		}
		defer deleteNamespace(w.Namespace)
		defer deleteConfig(config)
		var run Runner = func(ctx context.Context) error { return nil }
		for _, ip := range ips {
			run = run.Append(func(ctx context.Context) error {
				return client.Connect(ctx, a.PilotAddress, ip, meta)
			})
		}
		return run(ctx)
	}, nil
}

func createWorkload(w Workload) (string, []string) {
	ips := []string{}

	out := ""
	out = combineYaml(out, NamespaceSpec{
		Name: w.Namespace,
	}.Generate())
	out = combineYaml(out, ServiceAccountSpec{
		App:       w.App,
		Namespace: w.Namespace,
		Name:      w.ServiceAccount,
	}.Generate())

	for i := 0; i < w.Instances; i++ {
		ip := getIp()
		out = combineYaml(out, PodSpec{
			ServiceAccount: w.ServiceAccount,
			Node:           w.Node,
			App:            w.App,
			Namespace:      w.Namespace,
			IP:             ip,
		}.Generate())
		ips = append(ips, ip)
	}

	out = combineYaml(out, EndpointSpec{
		Node:      w.Node,
		App:       w.App,
		Namespace: w.Namespace,
		IPs:       ips,
	}.Generate())
	out = combineYaml(out, ServiceSpec{
		App:       w.App,
		Namespace: w.Namespace,
		IP:        getIp(),
	}.Generate())
	return out, ips
}
