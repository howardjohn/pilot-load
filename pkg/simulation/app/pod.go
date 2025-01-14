package app

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/credentials"
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/sleep"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
	"github.com/howardjohn/pilot-load/pkg/simulation/xds"
)

type PodSpec struct {
	ServiceAccount string
	Node           string
	App            string
	Namespace      string
	UID            string
	IP             string
	AppType        model.AppType
}

type Pod struct {
	Spec *PodSpec
	// For internal optimization around closing only
	created bool
	xds     *xds.Simulation
}

var _ model.Simulation = &Pod{}

func NewPod(s PodSpec) *Pod {
	if s.UID == "" {
		s.UID = util.GenUID()
	}
	if s.IP == "" {
		s.IP = util.GetIP()
	}
	return &Pod{
		Spec: &s,
	}
}

type GrpcCredentials struct {
	Metadata func() (map[string]string, error)
}

func (g GrpcCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return g.Metadata()
}

func (g GrpcCredentials) RequireTransportSecurity() bool {
	return true
}

var _ credentials.PerRPCCredentials = &GrpcCredentials{}

func (p *Pod) Run(ctx model.Context) (err error) {
	pod := p.getPod()

	{
		var terr error
		for range 10 {
			if err := kube.ApplyRealSSA(ctx.Client, pod); err != nil {
				// Sometimes there is a race with SA being created in the namespace...
				sleep.UntilContext(ctx, time.Millisecond*500)
				terr = fmt.Errorf("failed to apply pod: %v", err)
			} else {
				p.created = true
				break
			}
		}
		if !p.created {
			return terr
		}
	}

	if p.Spec.AppType.HasProxy() {
		p.xds = &xds.Simulation{
			Labels:    pod.Labels,
			Namespace: pod.Namespace,
			Name:      pod.Name,
			IP:        p.Spec.IP,
			AppType:   p.Spec.AppType,
			// TODO: multicluster
			Cluster:  "Kubernetes",
			GrpcOpts: ctx.Args.Auth.GrpcOptions(p.Spec.ServiceAccount, p.Spec.Namespace),
			Delta:    ctx.Args.DeltaXDS,
		}
		return p.xds.Run(ctx)
	} else {
		log.Infof("Starting pod %v", pod.Name)
	}
	return nil
}

func (p *Pod) Cleanup(ctx model.Context) error {
	if p.created {
		if err := kube.Delete(ctx.Client, p.getPod()); err != nil {
			return err
		}
	}
	if p.Spec.AppType.HasProxy() {
		return p.xds.Cleanup(ctx)
	}
	return nil
}

func (p *Pod) Name() string {
	return fmt.Sprintf("%s-%s", p.Spec.App, p.Spec.UID)
}

func (p *Pod) getPod() *v1.Pod {
	s := p.Spec
	labels := map[string]string{
		"app":                     s.App,
		"owner":                   "pilot-load",
		"sidecar.istio.io/inject": "false",
	}
	if p.Spec.AppType == model.SidecarType {
		labels["sidecar.istio.io/inject"] = "true"
	}

	annotations := map[string]string{
		"prometheus.io/scrape": "false",
	}
	if p.Spec.AppType == model.AmbientType {
		annotations["ambient.istio.io/redirection"] = "enabled"
	}
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        p.Name(),
			Namespace:   s.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: v1.PodSpec{
			TerminationGracePeriodSeconds: ptr.Of(int64(0)),
			ServiceAccountName:            s.ServiceAccount,
			Containers: []v1.Container{{
				Name:  "app",
				Image: "fake",
			}},
			// Schedule ourselves, kube scheduler is slow. TODO: make it optional?
			NodeName: s.Node,
			NodeSelector: map[string]string{
				"pilot-load.istio.io/node": "fake",
			},
			Tolerations: []v1.Toleration{{
				Key:      "pilot-load.istio.io/node",
				Operator: v1.TolerationOpExists,
				Effect:   v1.TaintEffectNoSchedule,
			}},
		},
	}
}
