package simulation

import (
	"fmt"

	"github.com/howardjohn/pilot-load/client"
)

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

type Pod struct {
	Spec *PodSpec
}

var _ Simulation = &Pod{}

func NewPod(s PodSpec) *Pod {
	if s.UID == "" {
		s.UID = genUID()
	}
	if s.IP == "" {
		s.IP = getIp()
	}
	return &Pod{
		Spec: &s,
	}
}

func (p *Pod) Run(ctx Context) (err error) {
	if err = applyConfig(render(podYml, p.Spec)); err != nil {
		return fmt.Errorf("failed to apply config: %v", err)
	}
	meta := map[string]interface{}{
		"ISTIO_VERSION": "1.5.0",
		"CLUSTER_ID":    "Kubernetes",
		"LABELS": map[string]string{
			"app": p.Spec.App,
		},
		"CONFIG_NAMESPACE": p.Spec.Namespace,
	}
	defer func() {
		err = deleteConfig(render(podYml, p.Spec))
	}()
	return client.Connect(ctx, ctx.args.PilotAddress, p.Spec.IP, meta)
}
