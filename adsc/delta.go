package adsc

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"unique"

	"istio.io/istio/pkg/util/protomarshal"

	"unique"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/grpc"

	v3 "istio.io/istio/pilot/pkg/xds/v3"
	"istio.io/istio/pkg/slices"
	"istio.io/istio/pkg/util/protomarshal"
	"istio.io/istio/pkg/util/sets"
)

type ResourceKey struct {
	Name    IString
	TypeUrl IString
}

func (k ResourceKey) String() string {
	if k == (ResourceKey{}) {
		return "<wildcard>"
	}
	return strings.TrimPrefix(k.TypeUrl.Value(), "type.googleapis.com/envoy.config.") + "/" + k.Name.Value()
}

type ResourceNode struct {
	Key ResourceKey

	Parents  sets.Set[*ResourceNode]
	Children sets.Set[*ResourceNode]
}

type (
	IString    = unique.Handle[string]
	IStringSet = sets.Set[IString]
)

type deltaClient struct {
	initialWatches []string
	node           *core.Node
	conn           *grpc.ClientConn
	client         discovery.AggregatedDiscoveryService_DeltaAggregatedResourcesClient

	// Updates includes the type of the last update received from the server.
	updates chan string

	mu        sync.Mutex
	resources map[IString]IStringSet
	tree      map[ResourceKey]*ResourceNode
}

var _ ADSClient = &deltaClient{}

func DialDelta(url string, opts *Config) (ADSClient, error) {
	ListenerNode := &ResourceNode{
		Key:      ResourceKey{TypeUrl: intern(v3.ListenerType)},
		Parents:  sets.New[*ResourceNode](),
		Children: sets.New[*ResourceNode](),
	}

	ClusterNode := &ResourceNode{
		Key:      ResourceKey{TypeUrl: intern(v3.ClusterType)},
		Parents:  sets.New[*ResourceNode](),
		Children: sets.New[*ResourceNode](),
	}
	nodeID := fmt.Sprintf("%s~%s~%s.%s~%s.svc.cluster.local", opts.NodeType, opts.IP,
		opts.Workload, opts.Namespace, opts.Namespace)

	conn, err := grpc.DialContext(opts.Context, url, opts.GrpcOpts...)
	if err != nil {
		return nil, fmt.Errorf("dial context: %v", err)
	}

	xds := discovery.NewAggregatedDiscoveryServiceClient(conn)
	xdsClient, err := xds.DeltaAggregatedResources(opts.Context, grpc.MaxCallRecvMsgSize(math.MaxInt32))
	if err != nil {
		return nil, fmt.Errorf("stream: %v", err)
	}
	c := &deltaClient{
		initialWatches: []string{v3.ClusterType, v3.ListenerType},
		node:           makeNode(nodeID, opts.Meta),
		conn:           conn,
		client:         xdsClient,
		resources:      map[IString]IStringSet{},
		tree: map[ResourceKey]*ResourceNode{
			ListenerNode.Key: ListenerNode,
			ClusterNode.Key:  ClusterNode,
		},
		updates: make(chan string, 100),
	}
	if opts.NodeType == "ztunnel" {
		c.initialWatches = []string{v3.AddressType, v3.WorkloadAuthorizationType}
	}
	go c.handleRecv()
	return c, nil
}

func (d *deltaClient) handleRecv() {
	scope := scope.WithLabels("node", d.node.Id)
	for {
		msg, err := d.client.Recv()
		if err != nil {
			scope.Infof("Connection closed: %v", err)
			d.Close()
			d.updates <- "close"
			return
		}

		requests := map[IString][]IString{}

		d.mu.Lock()
		typeUrl := intern(msg.TypeUrl)
		resources := d.resources[typeUrl]
		if resources == nil {
			resources = sets.NewWithLength[IString](len(msg.Resources))
		}
		origLen := len(d.resources[typeUrl])
		for _, resp := range msg.Resources {
			name := intern(resp.Name)
			key := ResourceKey{
				Name:    name,
				TypeUrl: typeUrl,
			}
			if d.tree[key] == nil && hasChildTypes(typeUrl.Value()) {
				d.tree[key] = &ResourceNode{
					Key:      key,
					Parents:  sets.New[*ResourceNode](),
					Children: sets.New[*ResourceNode](),
				}
				switch msg.TypeUrl {
				case v3.ListenerType:
					relate(d.tree[ResourceKey{TypeUrl: intern(v3.ListenerType)}], d.tree[key])
				case v3.ClusterType:
					relate(d.tree[ResourceKey{TypeUrl: intern(v3.ClusterType)}], d.tree[key])
				}

			} else if !isWildcardTypeURL(typeUrl.Value()) && d.tree[key] == nil {
				scope.Warnf("Ignoring unmatched resource %s", key)
				continue
			}
			node := d.tree[key]
			resources = resources.Insert(name)
			referenced := extractReferencedKeys(resp)
			for _, rkey := range referenced {
				child, f := d.getNode(rkey)
				if !f {
					requests[rkey.TypeUrl] = append(requests[rkey.TypeUrl], rkey.Name)
				}
				relate(node, child)
			}
		}
		removals := map[IString][]IString{}
		for _, resp := range msg.RemovedResources {
			name := intern(resp)
			key := ResourceKey{
				Name:    name,
				TypeUrl: typeUrl,
			}
			if d.tree[key] == nil {
				scope.Debugf("Ignoring removing unmatched resource %s", key)
				continue
			}
			node := d.tree[key]
			d.deleteNode(node, removals)
		}

		addedLen := len(resources) - origLen
		removedLen := len(msg.RemovedResources)
		for _, m := range msg.RemovedResources {
			resources.Delete(intern(m))
		}
		d.resources[typeUrl] = resources
		d.mu.Unlock()
		scope.WithLabels("type", msg.TypeUrl, "added", addedLen, "removed", removedLen, "removed refs", len(removals)).Debugf("got message")
		if dumpScope.DebugEnabled() {
			s, _ := protomarshal.ToJSON(msg)
			dumpScope.Debug(s)
		}
		if dumpScope.InfoEnabled() {
			d.mu.Lock()
			dumpScope.Info("\n" + d.dumpTree())
			d.mu.Unlock()
		}

		// TODO: Envoy does some smart "pausing" to allow the next push to come before we request
		for _, k := range keysOfMaps(requests, removals) {
			if len(requests[k]) == 0 && len(removals[k]) == 0 {
				continue
			}
			if err := d.send(&discovery.DeltaDiscoveryRequest{
				TypeUrl: k.Value(),
				ResourceNamesSubscribe: slices.Map(requests[k], func(e IString) string {
					return e.Value()
				}),
				ResourceNamesUnsubscribe: slices.Map(removals[k], func(e IString) string {
					return e.Value()
				}),
			}, ReasonRequest); err != nil {
				scope.Errorf("error sending request: %v", err)
			}
		}

		if err := d.send(&discovery.DeltaDiscoveryRequest{
			TypeUrl:       msg.TypeUrl,
			ResponseNonce: msg.Nonce,
		}, ReasonAck); err != nil {
			scope.Errorf("error sending ACK: %v", err)
		}
	}
}

func keysOfMaps(ms ...map[IString][]IString) []IString {
	res := []IString{}
	for _, m := range ms {
		for k := range m {
			res = append(res, k)
		}
	}
	return res
}

func extractReferencedKeys(resp *discovery.Resource) []ResourceKey {
	res := []ResourceKey{}
	switch resp.Resource.TypeUrl {
	case v3.ClusterType:
		o := &cluster.Cluster{}
		_ = resp.Resource.UnmarshalTo(o)
		switch v := o.GetClusterDiscoveryType().(type) {
		case *cluster.Cluster_Type:
			if v.Type != cluster.Cluster_EDS {
				return res
			}
		}
		key := ResourceKey{
			Name:    intern(o.Name),
			TypeUrl: intern(v3.EndpointType),
		}
		res = append(res, key)
	case v3.ListenerType:
		o := &listener.Listener{}
		_ = resp.Resource.UnmarshalTo(o)
		for _, fc := range getFilterChains(o) {
			for _, f := range fc.GetFilters() {
				if f.GetTypedConfig().GetTypeUrl() == "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager" {
					hcm := &hcm.HttpConnectionManager{}
					_ = f.GetTypedConfig().UnmarshalTo(hcm)
					if r := hcm.GetRds().GetRouteConfigName(); r != "" {
						key := ResourceKey{
							Name:    intern(r),
							TypeUrl: intern(v3.RouteType),
						}
						res = append(res, key)
					}
				}
			}
		}
	}
	return res
}

func relate(parent, child *ResourceNode) {
	parent.Children[child] = struct{}{}
	child.Parents[parent] = struct{}{}
}

func (d *deltaClient) Watch() {
	scope.Infof("sending initial watches")
	first := true
	for _, res := range d.initialWatches {
		req := &discovery.DeltaDiscoveryRequest{
			TypeUrl: res,
		}
		if first {
			req.Node = d.node
			first = false
		}
		err := d.send(req, ReasonInit)
		if err != nil {
			scope.Errorf("Error sending request: %v", err)
		}
	}
}

func (d *deltaClient) Close() {
	if d.conn != nil {
		d.conn.Close()
	}
}

func (d *deltaClient) Responses() Responses {
	panic("implement me")
}

func (d *deltaClient) Updates() chan string {
	return d.updates
}

func (d *deltaClient) send(dr *discovery.DeltaDiscoveryRequest, reason string) error {
	scope.Debugf("send message for type %v (%v) for +%v -%v", dr.TypeUrl, reason, dr.ResourceNamesSubscribe, dr.ResourceNamesUnsubscribe)
	return d.client.Send(dr)
}

func (d *deltaClient) getNode(key ResourceKey) (*ResourceNode, bool) {
	found := true
	if d.tree[key] == nil {
		d.tree[key] = &ResourceNode{
			Key:      key,
			Parents:  sets.New[*ResourceNode](),
			Children: sets.New[*ResourceNode](),
		}
		found = false
	}
	return d.tree[key], found
}

func (d *deltaClient) dumpTree() string {
	sb := strings.Builder{}
	roots := []*ResourceNode{}
	for _, n := range d.tree {
		if len(n.Parents) == 0 {
			roots = append(roots, n)
		}
	}
	for _, r := range roots {
		dumpNode(&sb, r, "")
	}
	return sb.String()
}

func (d *deltaClient) deleteNode(node *ResourceNode, removals map[IString][]IString) {
	delete(d.tree, node.Key)
	for p := range node.Parents {
		delete(p.Children, node)
	}
	for c := range node.Children {
		delete(c.Parents, node)
		removals[c.Key.TypeUrl] = append(removals[c.Key.TypeUrl], c.Key.Name)
		if len(c.Parents) == 0 {
			d.deleteNode(c, removals)
		}
	}
}

func dumpNode(sb *strings.Builder, node *ResourceNode, indent string) {
	sb.WriteString(indent + node.Key.String() + ":\n")
	if len(indent) > 10 {
		return
	}
	for c := range node.Children {
		id := indent + "  "
		if _, f := c.Parents[node]; !f {
			id = indent + "**"
		}
		dumpNode(sb, c, id)
	}
}

func hasChildTypes(typeURL string) bool {
	switch typeURL {
	case v3.SecretType, v3.EndpointType, v3.RouteType, v3.ExtensionConfigurationType:
		// By XDS spec, these are not wildcard
		return false
	case v3.WorkloadAuthorizationType, v3.AddressType, v3.WorkloadType:
		// These do not have children
		return false
	case v3.ClusterType, v3.ListenerType:
		// By XDS spec, these are wildcard
		return true
	default:
		// All of our internal types use wildcard semantics
		return true
	}
}

func isWildcardTypeURL(typeURL string) bool {
	switch typeURL {
	case v3.SecretType, v3.EndpointType, v3.RouteType, v3.ExtensionConfigurationType:
		// By XDS spec, these are not wildcard
		return false
	case v3.ClusterType, v3.ListenerType:
		// By XDS spec, these are wildcard
		return true
	default:
		// All of our internal types use wildcard semantics
		return true
	}
}

func intern(s string) unique.Handle[string] {
	return unique.Make(s)
}
