package dump

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	quicv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/quic/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"golang.org/x/exp/maps"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"istio.io/istio/pilot/pkg/util/protoconv"
	v3 "istio.io/istio/pilot/pkg/xds/v3"
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/util/protomarshal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/adsc"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/security"
)

type DumpSpec struct {
	Pod       string
	Namespace string

	OutputDir string
}

type DumpSimulation struct {
	Spec DumpSpec
	done []chan struct{}
}

var _ model.Simulation = &DumpSimulation{}

func NewSimulation(spec DumpSpec) *DumpSimulation {
	return &DumpSimulation{Spec: spec}
}

func (i *DumpSimulation) Run(ctx model.Context) error {
	pod, err := ctx.Client.Kube().CoreV1().Pods(i.Spec.Namespace).Get(context.Background(), i.Spec.Pod, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("pod not found: %v", err)
	}
	done := make(chan struct{})
	i.done = append(i.done, done)
	ip := pod.Status.PodIP

	proxyType := "sidecar"
	podMeta := map[string]string{}
	for _, c := range pod.Spec.Containers {
		if c.Name == "istio-proxy" && len(c.Args) > 2 && c.Args[1] == "router" {
			proxyType = "router"
		}
		for _, e := range c.Env {
			if strings.HasPrefix(e.Name, "ISTIO_META_") && e.Value != "" {
				podMeta[strings.TrimPrefix(e.Name, "ISTIO_META_")] = e.Value
			}
		}
	}

	meta := clone(ctx.Args.Metadata)
	meta["ISTIO_VERSION"] = "1.20.0-pilot-load"
	meta["LABELS"] = pod.Labels
	meta["NAMESPACE"] = pod.Namespace
	meta["SERVICE_ACCOUNT"] = pod.Spec.ServiceAccountName
	meta["NODE_NAME"] = pod.Spec.NodeName
	for k, v := range podMeta {
		meta[k] = v
	}

	resp, err := adsc.Fetch(ctx.Args.PilotAddress, &adsc.Config{
		Namespace:      pod.Namespace,
		Workload:       pod.Name,
		Meta:           meta,
		NodeType:       proxyType,
		IP:             ip,
		Context:        ctx,
		GrpcOpts:       ctx.Args.Auth.GrpcOptions(pod.Spec.ServiceAccountName, pod.Namespace),
		StoreResponses: true,
	})
	if err != nil {
		return err
	}
	log.Infof("response received")
	cert, err := ctx.Args.Auth.Certificate(ctx.Client.FetchRootCert, ctx.Args.PilotAddress, pod.Spec.ServiceAccountName, pod.Namespace)
	if err != nil {
		return fmt.Errorf("failed to create cert: %v", err)
	}
	log.Infof("certificate received")
	_ = cert
	return i.write(resp, cert)
}

func (i *DumpSimulation) write(resp *adsc.Responses, cert security.Cert) error {
	if i.Spec.OutputDir != "" {
		_ = os.MkdirAll(i.Spec.OutputDir, 0o777)
		_ = os.MkdirAll(i.Spec.OutputDir+"/rds", 0o777)
		_ = os.MkdirAll(i.Spec.OutputDir+"/eds", 0o777)
		_ = os.MkdirAll(i.Spec.OutputDir+"/sds", 0o777)
	}
	writeResponse(clusterResponse(i.Spec.OutputDir, transmute[*cluster.Cluster](resp.Clusters)), i.Spec.OutputDir, "cds.yaml")
	writeResponse(listenerResponse(i.Spec.OutputDir, transmute[*listener.Listener](resp.Listeners)), i.Spec.OutputDir, "lds.yaml")
	for name, rt := range resp.Routes {
		writeResponse(routesResponse([]*route.RouteConfiguration{rt.(*route.RouteConfiguration)}), i.Spec.OutputDir, fmt.Sprintf("rds/%s.yaml", SanitizeName(name)))
	}
	for name, ep := range resp.Endpoints {
		writeResponse(endpointsResponse([]*endpoint.ClusterLoadAssignment{ep.(*endpoint.ClusterLoadAssignment)}), i.Spec.OutputDir, fmt.Sprintf("eds/%s.yaml", SanitizeName(name)))
	}
	for name, s := range resp.Secrets {
		writeResponse(secretXdsResponse(i.Spec.OutputDir, []*tls.Secret{s.(*tls.Secret)}), i.Spec.OutputDir, fmt.Sprintf("sds/%s.yaml", SanitizeName(name)))
	}

	writeBytes(bootstrap(i.Spec.OutputDir), i.Spec.OutputDir, "config.yaml")

	writeResponse(secretResponse(i.Spec.OutputDir, cert), i.Spec.OutputDir, "sds/default.yaml")
	writeResponse(secretRootResponse(i.Spec.OutputDir, cert), i.Spec.OutputDir, "sds/ROOTCA.yaml")

	return nil
}

func (i *DumpSimulation) Cleanup(ctx model.Context) error {
	return nil
}

func clone(m map[string]string) map[string]interface{} {
	n := map[string]interface{}{}
	for k, v := range m {
		n[k] = v
	}
	return n
}

func transmute[T proto.Message](resp map[string]proto.Message) []T {
	keys := maps.Keys(resp)
	sort.Strings(keys)
	res := make([]T, 0, len(resp))
	for _, k := range keys {
		m := resp[k]
		res = append(res, m.(T))
	}
	return res
}

func endpointsResponse(response []*endpoint.ClusterLoadAssignment) *discovery.DiscoveryResponse {
	out := &discovery.DiscoveryResponse{
		TypeUrl:     v3.EndpointType,
		VersionInfo: "0",
	}

	for _, c := range response {
		cc, _ := anypb.New(c)
		out.Resources = append(out.Resources, cc)
	}

	return out
}

func clusterResponse(path string, response []*cluster.Cluster) *discovery.DiscoveryResponse {
	out := &discovery.DiscoveryResponse{
		TypeUrl:     v3.ClusterType,
		VersionInfo: "0",
	}

	sanitizeClusterAds(path, response)

	for _, c := range response {
		cc, _ := anypb.New(c)
		out.Resources = append(out.Resources, cc)
	}

	return out
}

func secretXdsResponse(path string, response []*tls.Secret) *discovery.DiscoveryResponse {
	out := &discovery.DiscoveryResponse{
		TypeUrl:     v3.SecretType,
		VersionInfo: "0",
	}

	sanitizeSecretAds(path, response)

	for _, c := range response {
		cc, _ := anypb.New(c)
		out.Resources = append(out.Resources, cc)
	}

	return out
}

func secretResponse(path string, cert security.Cert) *discovery.DiscoveryResponse {
	out := &discovery.DiscoveryResponse{
		TypeUrl:     v3.SecretType,
		VersionInfo: "0",
	}
	secret := &tls.Secret{
		Name: "default",
		Type: &tls.Secret_TlsCertificate{
			TlsCertificate: &tls.TlsCertificate{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: cert.ClientCert,
					},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: cert.Key,
					},
				},
			},
		},
	}
	cc, _ := anypb.New(secret)
	out.Resources = append(out.Resources, cc)

	return out
}

func secretRootResponse(path string, cert security.Cert) *discovery.DiscoveryResponse {
	out := &discovery.DiscoveryResponse{
		TypeUrl:     v3.SecretType,
		VersionInfo: "0",
	}
	secret := &tls.Secret{
		Name: "ROOTCA",
		Type: &tls.Secret_ValidationContext{
			ValidationContext: &tls.CertificateValidationContext{
				TrustedCa: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: cert.RootCert,
					},
				},
			},
		},
	}
	cc, _ := anypb.New(secret)
	out.Resources = append(out.Resources, cc)

	return out
}

func listenerResponse(path string, response []*listener.Listener) *discovery.DiscoveryResponse {
	out := &discovery.DiscoveryResponse{
		TypeUrl:     v3.ListenerType,
		VersionInfo: "0",
	}

	sanitizeListenerAds(path, response)

	for _, c := range response {
		cc, _ := anypb.New(c)
		out.Resources = append(out.Resources, cc)
	}

	return out
}

func routesResponse(response []*route.RouteConfiguration) *discovery.DiscoveryResponse {
	out := &discovery.DiscoveryResponse{
		TypeUrl:     v3.RouteType,
		VersionInfo: "0",
	}

	for _, c := range response {
		cc, _ := anypb.New(c)
		out.Resources = append(out.Resources, cc)
	}

	return out
}

func sanitizeClusterAds(path string, response []*cluster.Cluster) {
	for _, r := range response {
		rewriteTransportSocket(path, r.TransportSocket)
		for _, tsm := range r.TransportSocketMatches {
			rewriteTransportSocket(path, tsm.TransportSocket)
		}
		if r.EdsClusterConfig == nil {
			continue
		}
		path := filepath.Join(path, "eds", SanitizeName(r.Name)+".yaml")
		r.EdsClusterConfig.EdsConfig = toPath(path)
	}
}

func sanitizeSecretAds(path string, response []*tls.Secret) {
}

func rewriteTransportSocket(path string, s *core.TransportSocket) {
	if s == nil {
		return
	}
	if s.GetTypedConfig().TypeUrl == TypeName[*tls.DownstreamTlsContext]() {
		tl := SilentlyUnmarshalAny[tls.DownstreamTlsContext](s.GetTypedConfig())
		for _, sds := range tl.CommonTlsContext.TlsCertificateSdsSecretConfigs {
			sds.SdsConfig = toPath(filepath.Join(path, "sds", SanitizeName(sds.Name)+".yaml"))
		}
		if v := tl.CommonTlsContext.GetValidationContextSdsSecretConfig(); v != nil {
			v.SdsConfig = toPath(filepath.Join(path, "sds", SanitizeName(v.Name)+".yaml"))
		}
		if v := tl.CommonTlsContext.GetCombinedValidationContext().GetValidationContextSdsSecretConfig(); v != nil {
			v.SdsConfig = toPath(filepath.Join(path, "sds", SanitizeName(v.Name)+".yaml"))
		}
		s.ConfigType = &core.TransportSocket_TypedConfig{TypedConfig: protoconv.MessageToAny(tl)}
		return
	}
	if s.GetTypedConfig().TypeUrl == TypeName[*quicv3.QuicDownstreamTransport]() {
		tl := SilentlyUnmarshalAny[quicv3.QuicDownstreamTransport](s.GetTypedConfig())
		for _, sds := range tl.DownstreamTlsContext.CommonTlsContext.TlsCertificateSdsSecretConfigs {
			sds.SdsConfig = toPath(filepath.Join(path, "sds", SanitizeName(sds.Name)+".yaml"))
		}
		if v := tl.DownstreamTlsContext.CommonTlsContext.GetValidationContextSdsSecretConfig(); v != nil {
			v.SdsConfig = toPath(filepath.Join(path, "sds", SanitizeName(v.Name)+".yaml"))
		}
		if v := tl.DownstreamTlsContext.CommonTlsContext.GetCombinedValidationContext().GetValidationContextSdsSecretConfig(); v != nil {
			v.SdsConfig = toPath(filepath.Join(path, "sds", SanitizeName(v.Name)+".yaml"))
		}
		s.ConfigType = &core.TransportSocket_TypedConfig{TypedConfig: protoconv.MessageToAny(tl)}
		return
	}
	if s.GetTypedConfig().TypeUrl == TypeName[*tls.UpstreamTlsContext]() {
		tl := SilentlyUnmarshalAny[tls.UpstreamTlsContext](s.GetTypedConfig())
		for _, sds := range tl.CommonTlsContext.TlsCertificateSdsSecretConfigs {
			sds.SdsConfig = toPath(filepath.Join(path, "sds", SanitizeName(sds.Name)+".yaml"))
		}
		if v := tl.CommonTlsContext.GetValidationContextSdsSecretConfig(); v != nil {
			v.SdsConfig = toPath(filepath.Join(path, "sds", SanitizeName(v.Name)+".yaml"))
		}
		if v := tl.CommonTlsContext.GetCombinedValidationContext().GetValidationContextSdsSecretConfig(); v != nil {
			v.SdsConfig = toPath(filepath.Join(path, "sds", SanitizeName(v.Name)+".yaml"))
		}
		s.ConfigType = &core.TransportSocket_TypedConfig{TypedConfig: protoconv.MessageToAny(tl)}
	}
}

func sanitizeListenerAds(path string, response []*listener.Listener) {
	for _, c := range response {
		for _, fc := range filterChains(c) {
			rewriteTransportSocket(path, fc.TransportSocket)
			for _, f := range fc.Filters {
				if f.GetTypedConfig() == nil {
					continue
				}
				switch f.Name {
				case wellknown.HTTPConnectionManager:
					h := SilentlyUnmarshalAny[hcm.HttpConnectionManager](f.GetTypedConfig())
					switch r := h.GetRouteSpecifier().(type) {
					case *hcm.HttpConnectionManager_Rds:
						routeName := r.Rds.RouteConfigName
						h.RouteSpecifier = &hcm.HttpConnectionManager_Rds{Rds: &hcm.Rds{
							ConfigSource:    toPath(filepath.Join(path, "rds", SanitizeName(routeName)+".yaml")),
							RouteConfigName: "routeName",
						}}
						f.ConfigType = &listener.Filter_TypedConfig{TypedConfig: protoconv.MessageToAny(h)}
					}
				default:
				}
			}
		}
		// TODO proper ECDS
		keep := []*listener.ListenerFilter{}
		for _, f := range c.ListenerFilters {
			if f.GetConfigDiscovery() == nil {
				keep = append(keep, f)
			}
		}
		c.ListenerFilters = keep
	}
}

func toPath(p string) *core.ConfigSource {
	return &core.ConfigSource{
		ConfigSourceSpecifier: &core.ConfigSource_Path{Path: p},
	}
}

func filterChains(c *listener.Listener) []*listener.FilterChain {
	var chains []*listener.FilterChain
	chains = append(chains, c.FilterChains...)
	if c.DefaultFilterChain != nil {
		chains = append(chains, c.DefaultFilterChain)
	}
	return chains
}

func ExtractListenerNames(ll []*listener.Listener) []string {
	res := []string{}
	for _, l := range ll {
		res = append(res, l.Name)
	}
	return res
}

func SilentlyUnmarshalAny[T any](a *anypb.Any) *T {
	dst := any(new(T)).(proto.Message)
	if err := a.UnmarshalTo(dst); err != nil {
		var z *T
		return z
	}
	return any(dst).(*T)
}

func writeResponse(r *discovery.DiscoveryResponse, dir string, file string) {
	writeBytes(MarshallYaml(r), dir, file)
}

func writeBytes(yaml []byte, dir string, file string) {
	if dir == "" {
		fmt.Println(string(yaml))
	} else {
		if err := os.WriteFile(path.Join(dir, file), yaml, 0o777); err != nil {
			panic(err)
		}
	}
}

func bootstrap(outdir string) []byte {
	return []byte(fmt.Sprintf(`node:
  id: node
  cluster: envoy
admin:
  access_log_path: /dev/stdout
  address:
    socket_address:
      address: 0.0.0.0
      port_value: 15000
bootstrap_extensions:
- name: envoy.bootstrap.internal_listener
  typed_config:
    "@type": type.googleapis.com/udpa.type.v1.TypedStruct
    type_url: type.googleapis.com/envoy.extensions.bootstrap.internal_listener.v3.InternalListener
dynamic_resources:
  lds_config:
    path: %s/lds.yaml
  cds_config:
    path:  %s/cds.yaml`, outdir, outdir))
}

func MarshallYaml(w proto.Message) []byte {
	b, err := protomarshal.ToYAML(w)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %v:\n%+v", err, nil))
	}
	return []byte(b)
}

func SanitizeName(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name, "|", "_."), "/", "__.")
}

func TypeName[T proto.Message]() string {
	ft := new(T)
	return "type.googleapis.com/" + string((*ft).ProtoReflect().Descriptor().FullName())
}
