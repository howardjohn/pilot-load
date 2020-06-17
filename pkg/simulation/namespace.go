package simulation

import (
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
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

type Namespace struct {
	Spec *NamespaceSpec
}

var _ model.Simulation = &Namespace{}

func NewNamespace(s NamespaceSpec) *Namespace {
	return &Namespace{Spec: &s}
}

func (n Namespace) Run(ctx model.Context) (err error) {
	go func() {
		<-ctx.Done()
		err = util.AddError(err, deleteNamespace(n.Spec.Name))
	}()
	return RunConfig(ctx, func() string { return render(namespaceYml, n.Spec) })
}
