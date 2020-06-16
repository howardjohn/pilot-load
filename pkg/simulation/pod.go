package simulation

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/adsc"

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
	pod := p.GetPod()
	if err = ctx.client.Apply(pod); err != nil {
		return fmt.Errorf("failed to apply config: %v", err)
	}
	meta := map[string]interface{}{
		"ISTIO_VERSION": "1.6.0",
		"CLUSTER_ID":    "Kubernetes",
		"LABELS": map[string]string{
			"app": p.Spec.App,
		},
		"NAMESPACE": p.Spec.Namespace,
	}
	defer func() {
		err = AddError(err, ctx.client.Delete(pod))
	}()
	if err := client.Connect(ctx, ctx.args.PilotAddress, &adsc.Config{
		Namespace: p.Spec.Namespace,
		Workload:  fmt.Sprintf("%s-%s", p.Spec.App, p.Spec.UID),
		Meta:      meta,
		NodeType:  "sidecar",
		IP:        p.Spec.IP,
		Verbose:   false,
	}); err != nil {
		return fmt.Errorf("ads connection: %v", err)
	}
	return nil
}

func (p *Pod) GetPod() *v1.Pod {
	s := p.Spec
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", s.App, s.UID),
			Namespace: s.Namespace,
			Labels: map[string]string{
				"app": s.App,
			},
		},
		Spec: v1.PodSpec{
			Volumes: nil,
			InitContainers: []v1.Container{{
				Name:  "istio-init",
				Image: "istio/proxyv2",
			}},
			Containers: []v1.Container{{
				Name:  "app",
				Image: "app",
			}, {
				Name:  "istio-proxy",
				Image: "istio/proxyv2",
			}},
		},
		Status: v1.PodStatus{
			Phase:      v1.PodRunning,
			Conditions: nil,
			PodIP:      s.IP,
			PodIPs:     []v1.PodIP{{s.IP}},
		},
	}
}
