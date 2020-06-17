package simulation

import "github.com/howardjohn/pilot-load/pkg/simulation/model"

var (
	virtualServiceYml = `
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: {{.App}}
  namespace: {{.Namespace}}
spec:
  hosts:
  - {{.App}}
  http:
  - route:
    - destination:
        host: productpage
`
)

type VirtualServiceSpec struct {
	App       string
	Namespace string
}

type VirtualService struct {
	Spec *VirtualServiceSpec
}

// TODO migrate this over

func NewVirtualService(s VirtualServiceSpec) *VirtualService {
	return &VirtualService{Spec: &s}
}

func (v *VirtualService) Run(ctx model.Context) (err error) {
	return RunConfig(ctx, func() string { return render(virtualServiceYml, v.Spec) })
}
