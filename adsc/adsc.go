package adsc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"sort"
	"sync"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/golang/protobuf/jsonpb"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/util/sets"
)

var (
	scope     = log.RegisterScope("adsc", "")
	dumpScope = log.RegisterScope("dump", "")
)

var marshal = &jsonpb.Marshaler{OrigName: true, Indent: "  "}

// Config for the ADS connection.
type Config struct {
	// Namespace defaults to 'default'
	Namespace string

	// Workload defaults to 'test'
	Workload string

	// Meta includes additional metadata for the node
	Meta map[string]interface{}

	// NodeType defaults to sidecar. "ingress" and "router" are also supported.
	NodeType string

	// IP is currently the primary key used to locate inbound configs. It is sent by client,
	// must match a known endpoint IP. Tests can use a ServiceEntry to register fake IPs.
	IP string

	// Context used for early cancellation
	Context context.Context

	GrpcOpts []grpc.DialOption

	// Channel to report events back on
	Updates chan string
	Delta   bool

	StoreResponses bool
}

// ADSC implements a basic client for ADS, for use in stress tests and tools
// or libraries that need to connect to Istio pilot or other ADS servers.
type ADSC struct {
	ctx context.Context
	// Stream is the GRPC connection stream, allowing direct GRPC send operations.
	// Set after Dial is called.
	stream discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient

	conn *grpc.ClientConn

	// NodeID is the node identity sent to Pilot.
	nodeID string
	node   *core.Node

	done chan error

	GrpcOpts []grpc.DialOption

	url string

	InitialLoad bool

	// Metadata has the node metadata to send to pilot.
	// If nil, the defaults will be used.
	Metadata map[string]interface{}

	// Responses we received last
	responses Responses

	// Updates includes the type of the last update received from the server.
	updates chan string

	mutex   sync.Mutex
	watches map[string]Watch
	store   bool
}

func (a *ADSC) Updates() chan string {
	return a.updates
}

func (a *ADSC) Responses() Responses {
	return a.responses
}

type Responses struct {
	Clusters   map[string]proto.Message
	Listeners  map[string]proto.Message
	Routes     map[string]proto.Message
	Endpoints  map[string]proto.Message
	Extensions map[string]proto.Message
	Secrets    map[string]proto.Message
}

type Watch struct {
	resources   []string
	lastNonce   string
	lastVersion string
}

// ErrTimeout is returned by Wait if no update is received in the given time.
var ErrTimeout = errors.New("timeout")

type ADSClient interface {
	Watch()
	Updates() chan string
	Close()
	Responses() Responses
}

var _ ADSClient = &ADSC{}

// Dial connects to a ADS server, with optional MTLS authentication if a cert dir is specified.
func Dial(url string, opts *Config) (ADSClient, error) {
	if opts.Namespace == "" {
		opts.Namespace = "default"
	}
	if opts.NodeType == "" {
		opts.NodeType = "sidecar"
	}
	if opts.IP == "" {
		opts.IP = getPrivateIPIfAvailable().String()
	}
	if opts.Workload == "" {
		opts.Workload = "test-1"
	}
	if opts.Delta {
		return DialDelta(url, opts)
	}
	adsc := &ADSC{
		done:    make(chan error),
		updates: make(chan string, 100),
		watches: map[string]Watch{},
		responses: Responses{
			Clusters:   map[string]proto.Message{},
			Listeners:  map[string]proto.Message{},
			Routes:     map[string]proto.Message{},
			Endpoints:  map[string]proto.Message{},
			Secrets:    map[string]proto.Message{},
			Extensions: map[string]proto.Message{},
		},
		GrpcOpts: opts.GrpcOpts,
		url:      url,
		store:    opts.StoreResponses,
		ctx:      opts.Context,
	}

	adsc.Metadata = opts.Meta

	adsc.nodeID = fmt.Sprintf("%s~%s~%s.%s~%s.svc.cluster.local", opts.NodeType, opts.IP,
		opts.Workload, opts.Namespace, opts.Namespace)
	adsc.node = makeNode(adsc.nodeID, adsc.Metadata)
	if dumpScope.DebugEnabled() {
		n, _ := marshal.MarshalToString(adsc.node)
		dumpScope.Debugf("constructed node: %v", n)
	}
	err := adsc.Run()
	return adsc, err
}

// Returns a private IP address, or unspecified IP (0.0.0.0) if no IP is available
func getPrivateIPIfAvailable() net.IP {
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		default:
			continue
		}
		if !ip.IsLoopback() {
			return ip
		}
	}
	return net.IPv4zero
}

// Close the stream.
func (a *ADSC) Close() {
	a.mutex.Lock()
	if a.stream != nil {
		_ = a.stream.CloseSend()
	}
	if a.conn != nil {
		a.conn.Close()
	}
	a.mutex.Unlock()
}

// Run will run the ADS client.
func (a *ADSC) Run() error {
	var err error

	a.conn, err = grpc.DialContext(a.ctx, a.url, a.GrpcOpts...)
	if err != nil {
		return fmt.Errorf("dial context: %v", err)
	}

	xds := discovery.NewAggregatedDiscoveryServiceClient(a.conn)
	edsstr, err := xds.StreamAggregatedResources(a.ctx, grpc.MaxCallRecvMsgSize(math.MaxInt32))
	if err != nil {
		return fmt.Errorf("stream: %v", err)
	}
	a.stream = edsstr
	go a.handleRecv()
	return nil
}

func (a *ADSC) handleRecv() {
	for {
		msg, err := a.stream.Recv()
		if err != nil {
			scope.Infof("Connection closed for %v: %v", a.nodeID, err)
			a.Close()
			a.WaitClear()
			a.updates <- "close"
			return
		}
		scope.Debugf("got message for type %v", msg.TypeUrl)

		listeners := []*listener.Listener{}
		clusters := []*cluster.Cluster{}
		routes := []*route.RouteConfiguration{}
		eds := []*endpoint.ClusterLoadAssignment{}
		secrets := []*tls.Secret{}
		ecds := []*core.TypedExtensionConfig{}
		// TODO re-enable use of names. For now its skipped
		names := []string{}
		resp := map[string]proto.Message{}
		for _, rsc := range msg.Resources {
			valBytes := rsc.Value
			if rsc.TypeUrl == resource.ListenerType {
				ll := &listener.Listener{}
				_ = proto.Unmarshal(valBytes, ll)
				listeners = append(listeners, ll)
				if a.store {
					resp[ll.Name] = ll
				}
			} else if rsc.TypeUrl == resource.ClusterType {
				ll := &cluster.Cluster{}
				_ = proto.Unmarshal(valBytes, ll)
				clusters = append(clusters, ll)
				if a.store {
					resp[ll.Name] = ll
				}
			} else if rsc.TypeUrl == resource.EndpointType {
				ll := &endpoint.ClusterLoadAssignment{}
				_ = proto.Unmarshal(valBytes, ll)
				eds = append(eds, ll)
				names = append(names, ll.ClusterName)
				if a.store {
					resp[ll.ClusterName] = ll
				}
			} else if rsc.TypeUrl == resource.RouteType {
				ll := &route.RouteConfiguration{}
				_ = proto.Unmarshal(valBytes, ll)
				routes = append(routes, ll)
				names = append(names, ll.Name)
				if a.store {
					resp[ll.Name] = ll
				}
			} else if rsc.TypeUrl == resource.SecretType {
				ll := &tls.Secret{}
				_ = proto.Unmarshal(valBytes, ll)
				secrets = append(secrets, ll)
				names = append(names, ll.Name)
				if a.store {
					resp[ll.Name] = ll
				}
			} else if rsc.TypeUrl == resource.ExtensionConfigType {
				ll := &core.TypedExtensionConfig{}
				_ = proto.Unmarshal(valBytes, ll)
				ecds = append(ecds, ll)
				names = append(names, ll.Name)
				if a.store {
					resp[ll.Name] = ll
				}

			}
		}

		a.mutex.Lock()
		switch msg.TypeUrl {
		case resource.ListenerType:
			a.responses.Listeners = resp
		case resource.ClusterType:
			a.responses.Clusters = resp
		case resource.EndpointType:
			a.responses.Endpoints = resp
		case resource.RouteType:
			a.responses.Routes = resp
		case resource.SecretType:
			a.responses.Secrets = resp
		case resource.ExtensionConfigType:
			a.responses.Extensions = resp
		}
		a.ack(msg, names)
		a.mutex.Unlock()

		switch msg.TypeUrl {
		case resource.ListenerType:
			a.handleLDS(listeners)
		case resource.ClusterType:
			a.handleCDS(clusters)
		case resource.EndpointType:
			a.handleEDS(eds)
		case resource.RouteType:
			a.handleRDS(routes)
		case resource.SecretType:
			a.handleSDS(secrets)
		case resource.ExtensionConfigType:
			a.handleECDS(ecds)
		}
	}
}

func getFilterChains(l *listener.Listener) []*listener.FilterChain {
	chains := l.FilterChains
	if l.DefaultFilterChain != nil {
		chains = append(chains, l.DefaultFilterChain)
	}
	return chains
}

// nolint: staticcheck
func (a *ADSC) handleLDS(ll []*listener.Listener) {
	routes := []string{}
	extensions := sets.New[string]()
	sockets := []*core.TransportSocket{}
	secrets := sets.New[string]()
	for _, l := range ll {
		for _, fc := range getFilterChains(l) {
			for _, f := range fc.GetFilters() {
				if f.GetTypedConfig().GetTypeUrl() == "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager" {
					hcm := &hcm.HttpConnectionManager{}
					_ = f.GetTypedConfig().UnmarshalTo(hcm)
					if r := hcm.GetRds().GetRouteConfigName(); r != "" {
						routes = append(routes, r)
					}
					for _, f := range hcm.GetHttpFilters() {
						if f.GetConfigDiscovery() != nil {
							extensions.Insert(f.GetName())
						}
					}
				}
				if f.GetConfigDiscovery() != nil {
					extensions.Insert(f.GetName())
				}
			}
			if fc.GetTransportSocket() != nil {
				sockets = append(sockets, fc.GetTransportSocket())
			}
		}
		if ts := l.GetDefaultFilterChain().GetTransportSocket(); ts != nil {
			sockets = append(sockets, ts)
		}
	}
	for _, s := range sockets {
		dtl := &tls.DownstreamTlsContext{}
		_ = s.GetTypedConfig().UnmarshalTo(dtl)
		secrets.Insert(dtl.GetCommonTlsContext().GetCombinedValidationContext().GetValidationContextSdsSecretConfig().GetName())
		for _, s := range dtl.GetCommonTlsContext().GetTlsCertificateSdsSecretConfigs() {
			secrets.Insert(s.GetName())
		}
	}
	secrets.Delete("")
	secrets.Delete("ROOTCA")
	secrets.Delete("default")
	sort.Strings(routes)

	if dumpScope.DebugEnabled() {
		for i, l := range ll {
			b, err := marshal.MarshalToString(l)
			if err != nil {
				dumpScope.Errorf("Error in LDS: %v", err)
			}

			dumpScope.Debugf("lds %d: %v", i, b)
		}
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	if len(secrets) > 0 {
		a.handleResourceUpdate(resource.SecretType, sets.SortedList(secrets))
	}

	if len(extensions) > 0 {
		a.handleResourceUpdate(resource.ExtensionConfigType, sets.SortedList(extensions))
	}
	a.handleResourceUpdate(resource.RouteType, routes)

	select {
	case a.updates <- "lds":
	default:
	}
}

func (a *ADSC) handleResourceUpdate(typeUrl string, resources []string) {
	if !listEqual(a.watches[typeUrl].resources, resources) {
		scope.Debugf("%v type resources changed: %v -> %v", typeUrl, a.watches[typeUrl].resources, resources)
		watch := a.watches[typeUrl]
		watch.resources = resources
		a.watches[typeUrl] = watch
		a.request(typeUrl, watch)
	}
}

// listEqual checks that two lists contain all the same elements
func listEqual(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// compact representations, for simplified debugging/testing

// TCPListener extracts the core elements from envoy Listener.
type TCPListener struct {
	// Address is the address, as expected by go Dial and Listen, including port
	Address string

	// LogFile is the access log address for the listener
	LogFile string

	// Target is the destination cluster.
	Target string
}

type Target struct {
	// Address is a go address, extracted from the mangled cluster name.
	Address string

	// Endpoints are the resolved endpoints from EDS or cluster static.
	Endpoints map[string]Endpoint
}

type Endpoint struct {
	// Weight extracted from EDS
	Weight int
}

func (a *ADSC) handleCDS(ll []*cluster.Cluster) {
	cn := []string{}
	for _, c := range ll {
		switch v := c.ClusterDiscoveryType.(type) {
		case *cluster.Cluster_Type:
			if v.Type != cluster.Cluster_EDS {
				continue
			}
		}

		cn = append(cn, c.Name)
	}
	sort.Strings(cn)

	a.handleResourceUpdate(resource.EndpointType, cn)

	if dumpScope.DebugEnabled() {
		for i, c := range ll {
			b, err := marshal.MarshalToString(c)
			if err != nil {
				dumpScope.Errorf("Error in CDS: %v", err)
			}

			dumpScope.Debugf("cds %d: %v", i, b)
		}
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()
	if !a.InitialLoad {
		// first load - Envoy loads listeners after endpoints
		_ = a.send(&discovery.DiscoveryRequest{
			Node:    a.node,
			TypeUrl: resource.ListenerType,
		}, ReasonInit)
		a.InitialLoad = true
	}

	select {
	case a.updates <- "cds":
	default:
	}
}

func makeNode(id string, metadata interface{}) *core.Node {
	n := &core.Node{
		Id: id,
	}
	js, err := json.Marshal(metadata)
	if err != nil {
		panic("invalid metadata " + err.Error())
	}

	meta := &structpb.Struct{}
	err = jsonpb.UnmarshalString(string(js), meta)
	if err != nil {
		panic("invalid metadata " + err.Error())
	}

	n.Metadata = meta

	return n
}

func (a *ADSC) handleEDS(eds []*endpoint.ClusterLoadAssignment) {
	if dumpScope.DebugEnabled() {
		for i, e := range eds {
			b, err := marshal.MarshalToString(e)
			if err != nil {
				dumpScope.Errorf("Error in EDS: %v", err)
			}

			dumpScope.Debugf("eds %d: %v", i, b)
		}
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	select {
	case a.updates <- "eds":
	default:
	}
}

func (a *ADSC) handleRDS(configurations []*route.RouteConfiguration) {
	rds := map[string]*route.RouteConfiguration{}

	for _, r := range configurations {
		rds[r.Name] = r
	}

	if dumpScope.DebugEnabled() {
		for i, r := range configurations {
			b, err := marshal.MarshalToString(r)
			if err != nil {
				dumpScope.Errorf("Error in RDS: %v", err)
			}

			dumpScope.Debugf("rds %d: %v", i, b)
		}
	}

	select {
	case a.updates <- "rds":
	default:
	}
}

func (a *ADSC) handleSDS(configurations []*tls.Secret) {
	sds := map[string]*tls.Secret{}

	for _, r := range configurations {
		sds[r.Name] = r
	}

	if dumpScope.DebugEnabled() {
		for i, r := range configurations {
			b, err := marshal.MarshalToString(r)
			if err != nil {
				dumpScope.Errorf("Error in SDS: %v", err)
			}

			dumpScope.Debugf("sds %d: %v", i, b)
		}
	}

	select {
	case a.updates <- "sds":
	default:
	}
}

func (a *ADSC) handleECDS(configurations []*core.TypedExtensionConfig) {
	ecds := map[string]*core.TypedExtensionConfig{}

	for _, r := range configurations {
		ecds[r.Name] = r
	}

	if dumpScope.DebugEnabled() {
		for i, r := range configurations {
			b, err := marshal.MarshalToString(r)
			if err != nil {
				dumpScope.Errorf("Error in ECDS: %v", err)
			}

			dumpScope.Debugf("ecds %d: %v", i, b)
		}
	}

	select {
	case a.updates <- "ecds":
	default:
	}
}

// WaitClear will clear the waiting events, so next call to Wait will get
// the next push type.
func (a *ADSC) WaitClear() {
	for {
		select {
		case <-a.updates:
		default:
			return
		}
	}
}

// Wait for an update of the specified type. If type is empty, wait for next update.
func (a *ADSC) Wait(update string, to time.Duration) (string, error) {
	t := time.NewTimer(to)

	for {
		select {
		case t := <-a.updates:
			if len(update) == 0 || update == t {
				return t, nil
			}
		case <-t.C:
			return "", ErrTimeout
		}
	}
}

func (a *ADSC) send(dr *discovery.DiscoveryRequest, reason string) error {
	scope.Debugf("send message for type %v (%v) for %v", dr.TypeUrl, reason, dr.ResourceNames)
	return a.stream.Send(dr)
}

// Watch will start watching resources, starting with CDS. Based on the CDS response
// it will start watching RDS and CDS.
func (a *ADSC) Watch() {
	err := a.send(&discovery.DiscoveryRequest{
		Node:    a.node,
		TypeUrl: resource.ClusterType,
	}, ReasonInit)
	if err != nil {
		scope.Errorf("Error sending request: %v", err)
	}
}

const (
	ReasonAck     = "ack"
	ReasonRequest = "request"
	ReasonInit    = "init"
)

func (a *ADSC) request(typeUrl string, watch Watch) {
	_ = a.send(&discovery.DiscoveryRequest{
		ResponseNonce: watch.lastNonce,
		TypeUrl:       typeUrl,
		Node:          a.node,
		VersionInfo:   watch.lastVersion,
		ResourceNames: watch.resources,
	}, ReasonRequest)
}

func (a *ADSC) ack(msg *discovery.DiscoveryResponse, names []string) {
	watch := a.watches[msg.TypeUrl]
	watch.lastNonce = msg.Nonce
	watch.lastVersion = msg.VersionInfo
	a.watches[msg.TypeUrl] = watch
	// Incremental EDS can send partial endpoints, but in ack envoy should ack for all.
	if msg.TypeUrl == resource.EndpointType {
		names = watch.resources
	}
	_ = a.send(&discovery.DiscoveryRequest{
		ResponseNonce: msg.Nonce,
		TypeUrl:       msg.TypeUrl,
		Node:          a.node,
		VersionInfo:   msg.VersionInfo,
		ResourceNames: names,
	}, ReasonAck)
}
