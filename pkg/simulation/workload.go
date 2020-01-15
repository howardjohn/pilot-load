package simulation

import "fmt"

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

func generatePod(s PodSpec) string {
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

func CreateWorkload() {
	app := "foo"
	node := "node1"
	ns := "default"
	san := "sa"
	ip := getIp()

	sa := ServiceAccountSpec{
		App:       app,
		Namespace: ns,
		Name:      san,
	}.Generate()
	pod := generatePod(PodSpec{
		ServiceAccount: san,
		Node:           node,
		App:            app,
		Namespace:      ns,
		IP:             ip,
	})
	svc := ServiceSpec{
		App:       app,
		Namespace: ns,
		IP:        getIp(),
	}.Generate()
	ep := EndpointSpec{
		Node:      node,
		App:       app,
		Namespace: ns,
		IPs:       []string{ip},
	}.Generate()

	workload := combineYaml(sa, pod, svc, ep)
	fmt.Println(workload)
}
