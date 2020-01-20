package simulation

import (
	"fmt"
	"reflect"
)

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

type Endpoint struct {
	Spec    *EndpointSpec
	running bool
}

var _ Simulation = &Endpoint{}

func NewEndpoint(s EndpointSpec) *Endpoint {
	return &Endpoint{Spec: &s}
}

func (e Endpoint) SetAddresses(ips []string) error {
	if reflect.DeepEqual(e.Spec.IPs, ips) {
		return nil
	}
	e.Spec.IPs = ips
	if e.running {
		if err := applyConfig(render(endpointsYml, e.Spec)); err != nil {
			return fmt.Errorf("failed to apply config: %v", err)
		}
	}
	return nil
}

func (e *Endpoint) Run(ctx Context) (err error) {
	e.running = true
	return RunConfig(ctx, func() string { return render(endpointsYml, e.Spec) })
}
