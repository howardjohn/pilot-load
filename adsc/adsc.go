package adsc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"sync"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	"istio.io/pkg/log"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var scope = log.RegisterScope("adsc", "", 0)
var dumpScope = log.RegisterScope("dump", "", 0)

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

	certDir string
	url     string

	InitialLoad bool

	// Metadata has the node metadata to send to pilot.
	// If nil, the defaults will be used.
	Metadata map[string]interface{}

	// Updates includes the type of the last update received from the server.
	Updates chan string

	mutex sync.Mutex
}

var (
	// ErrTimeout is returned by Wait if no update is received in the given time.
	ErrTimeout = errors.New("timeout")
)

// Dial connects to a ADS server, with optional MTLS authentication if a cert dir is specified.
func Dial(url string, certDir string, opts *Config) (*ADSC, error) {
	adsc := &ADSC{
		done:    make(chan error),
		Updates: make(chan string, 100),
		certDir: certDir,
		url:     url,
		ctx:     opts.Context,
	}
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
	adsc.Metadata = opts.Meta

	adsc.nodeID = fmt.Sprintf("%s~%s~%s.%s~%s.svc.cluster.local", opts.NodeType, opts.IP,
		opts.Workload, opts.Namespace, opts.Namespace)
	adsc.node = adsc.makeNode()
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

func tlsConfig(certDir string) (*tls.Config, error) {
	clientCert, err := tls.LoadX509KeyPair(certDir+"/cert-chain.pem",
		certDir+"/key.pem")
	if err != nil {
		return nil, err
	}

	serverCABytes, err := ioutil.ReadFile(certDir + "/root-cert.pem")
	if err != nil {
		return nil, err
	}
	serverCAs := x509.NewCertPool()
	if ok := serverCAs.AppendCertsFromPEM(serverCABytes); !ok {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      serverCAs,
		ServerName:   "istio-pilot.istio-system",
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			return nil
		},
	}, nil
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
	if len(a.certDir) > 0 {
		tlsCfg, err := tlsConfig(a.certDir)
		if err != nil {
			return err
		}
		creds := credentials.NewTLS(tlsCfg)

		opts := []grpc.DialOption{
			// Verify Pilot cert and service account
			grpc.WithTransportCredentials(creds),
		}
		a.conn, err = grpc.Dial(a.url, opts...)
		if err != nil {
			return err
		}
	} else {
		a.conn, err = grpc.DialContext(a.ctx, a.url, grpc.WithInsecure())
		if err != nil {
			return err
		}
	}

	xds := discovery.NewAggregatedDiscoveryServiceClient(a.conn)
	edsstr, err := xds.StreamAggregatedResources(a.ctx)
	if err != nil {
		return err
	}
	a.stream = edsstr
	go a.handleRecv()
	return nil
}

func (a *ADSC) handleRecv() {
	for {
		msg, err := a.stream.Recv()
		if err != nil {
			scope.Debugf("Connection closed for %v: %v", a.nodeID, err)
			a.Close()
			a.WaitClear()
			a.Updates <- "close"
			return
		}
		scope.Debugf("got message for type %v", msg.TypeUrl)

		listeners := []*listener.Listener{}
		clusters := []*cluster.Cluster{}
		routes := []*route.RouteConfiguration{}
		eds := []*endpoint.ClusterLoadAssignment{}
		// TODO re-enable use of names. For now its skipped
		names := []string{}
		for _, rsc := range msg.Resources {
			valBytes := rsc.Value
			if rsc.TypeUrl == resource.ListenerType {
				ll := &listener.Listener{}
				_ = proto.Unmarshal(valBytes, ll)
				listeners = append(listeners, ll)
				names = append(names, ll.Name)
			} else if rsc.TypeUrl == resource.ClusterType {
				ll := &cluster.Cluster{}
				_ = proto.Unmarshal(valBytes, ll)
				clusters = append(clusters, ll)
				names = append(names, ll.Name)
			} else if rsc.TypeUrl == resource.EndpointType {
				// As an optimization. We don't need to inspect these like LDS/CDS
				if dumpScope.DebugEnabled() {
					ll := &endpoint.ClusterLoadAssignment{}
					_ = proto.Unmarshal(valBytes, ll)
					eds = append(eds, ll)
					names = append(names, ll.ClusterName)
				}
			} else if rsc.TypeUrl == resource.RouteType {
				if dumpScope.DebugEnabled() {
					ll := &route.RouteConfiguration{}
					_ = proto.Unmarshal(valBytes, ll)
					routes = append(routes, ll)
					names = append(names, ll.Name)
				}
			}
		}

		// TODO: add hook to inject nacks
		a.mutex.Lock()
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
		}
	}

}

// nolint: staticcheck
func (a *ADSC) handleLDS(ll []*listener.Listener) {
	routes := []string{}
	for _, l := range ll {
		f0 := l.FilterChains[0].Filters[0]
		if f0.Name == "envoy.http_connection_manager" {

			// Getting from config is too painful..
			port := l.Address.GetSocketAddress().GetPortValue()
			if port == 15002 {
				routes = append(routes, "http_proxy")
			} else {
				routes = append(routes, fmt.Sprintf("%d", port))
			}
		}
	}

	if dumpScope.DebugEnabled() {
		for i, l := range ll {
			b, err := marshal.MarshalToString(l)
			if err != nil {
				dumpScope.Errorf("Error in LDS: %v", err)
			}

			dumpScope.Debugf("%d: %v", i, b)
		}
	}
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if len(routes) > 0 {
		a.sendRequest(resource.RouteType, routes)
	}

	select {
	case a.Updates <- "lds":
	default:
	}
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

	if len(cn) > 0 {
		a.sendRequest(resource.EndpointType, cn)
	}
	if dumpScope.DebugEnabled() {
		for i, c := range ll {
			b, err := marshal.MarshalToString(c)
			if err != nil {
				dumpScope.Errorf("Error in CDS: %v", err)
			}

			dumpScope.Debugf("%d: %v", i, b)
		}
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	select {
	case a.Updates <- "cds":
	default:
	}
}

func (a *ADSC) makeNode() *core.Node {
	n := &core.Node{
		Id: a.nodeID,
	}
	js, err := json.Marshal(a.Metadata)
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

			dumpScope.Debugf("%d: %v", i, b)
		}
	}
	if !a.InitialLoad {
		// first load - Envoy loads listeners after endpoints
		_ = a.send(&discovery.DiscoveryRequest{
			ResponseNonce: time.Now().String(),
			Node:          a.node,
			TypeUrl:       resource.ListenerType,
		}, "init")
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	select {
	case a.Updates <- "eds":
	default:
	}
}

func (a *ADSC) handleRDS(configurations []*route.RouteConfiguration) {

	rds := map[string]*route.RouteConfiguration{}

	for _, r := range configurations {
		rds[r.Name] = r
	}

	if !a.InitialLoad {
		a.InitialLoad = true
	}

	if dumpScope.DebugEnabled() {
		for i, r := range configurations {
			b, err := marshal.MarshalToString(r)
			if err != nil {
				dumpScope.Errorf("Error in RDS: %v", err)
			}

			dumpScope.Debugf("%d: %v", i, b)
		}
	}

	select {
	case a.Updates <- "rds":
	default:
	}

}

// WaitClear will clear the waiting events, so next call to Wait will get
// the next push type.
func (a *ADSC) WaitClear() {
	for {
		select {
		case <-a.Updates:
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
		case t := <-a.Updates:
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
		ResponseNonce: time.Now().String(),
		Node:          a.node,
		TypeUrl:       resource.ClusterType,
	}, "init")
	if err != nil {
		scope.Errorf("Error sending request: ", err)
	}
}

func (a *ADSC) sendRequest(typeurl string, rsc []string) {
	_ = a.send(&discovery.DiscoveryRequest{
		ResponseNonce: "",
		Node:          a.node,
		TypeUrl:       typeurl,
		ResourceNames: rsc,
	}, "request")
}

func (a *ADSC) ack(msg *discovery.DiscoveryResponse, names []string) {
	sendNames := names
	// Pilot currently breaks if we do this properly.. send only routes
	if msg.TypeUrl != resource.RouteType {
		sendNames = []string{}
	}
	_ = a.send(&discovery.DiscoveryRequest{
		ResponseNonce: msg.Nonce,
		TypeUrl:       msg.TypeUrl,
		Node:          a.node,
		VersionInfo:   msg.VersionInfo,
		ResourceNames: sendNames,
	}, "ack")
}
