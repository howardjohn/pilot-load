package adsc

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/grpc"

	"istio.io/istio/pilot/pkg/util/sets"
	v3 "istio.io/istio/pilot/pkg/xds/v3"
)

type ResourceKey struct {
	Name    string
	TypeUrl string
}

func (k ResourceKey) String() string {
	if k == (ResourceKey{}) {
		return "<wildcard>"
	}
	return strings.TrimPrefix(k.TypeUrl, "type.googleapis.com/envoy.config.") + "/" + k.Name
}

type ResourceNode struct {
	Key ResourceKey

	Parents  map[*ResourceNode]struct{}
	Children map[*ResourceNode]struct{}
}

var ListenerNode = &ResourceNode{
	Key:      ResourceKey{TypeUrl: v3.ListenerType},
	Parents:  map[*ResourceNode]struct{}{},
	Children: map[*ResourceNode]struct{}{},
}

var ClusterNode = &ResourceNode{
	Key:      ResourceKey{TypeUrl: v3.ClusterType},
	Parents:  map[*ResourceNode]struct{}{},
	Children: map[*ResourceNode]struct{}{},
}

type deltaClient struct {
	initialWatches []string
	node           *core.Node
	conn           *grpc.ClientConn
	client         discovery.AggregatedDiscoveryService_DeltaAggregatedResourcesClient

	mu        sync.Mutex
	resources map[string]sets.Set
	tree      map[ResourceKey]*ResourceNode
}

var _ ADSClient = &deltaClient{}

func DialDelta(url string, opts *Config) (ADSClient, error) {
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
		resources:      map[string]sets.Set{},
		tree: map[ResourceKey]*ResourceNode{
			ListenerNode.Key: ListenerNode,
			ClusterNode.Key:  ClusterNode,
		},
	}
	go c.handleRecv()
	return c, nil
}

func (d *deltaClient) handleRecv() {
	for {
		msg, err := d.client.Recv()
		if err != nil {
			scope.Infof("Connection closed for %v: %v", d.node.Id, err)
			d.Close()
			return
		}

		requests := map[string][]string{}
		resources := sets.NewSet()
		for _, resp := range msg.Resources {
			key := ResourceKey{
				Name:    resp.Name,
				TypeUrl: msg.TypeUrl,
			}
			if d.tree[key] == nil && isWildcardTypeURL(msg.TypeUrl) {
				d.tree[key] = &ResourceNode{
					Key:      key,
					Parents:  map[*ResourceNode]struct{}{},
					Children: map[*ResourceNode]struct{}{},
				}
				switch msg.TypeUrl {
				case v3.ListenerType:
					relate(ListenerNode, d.tree[key])
				case v3.ClusterType:
					relate(ClusterNode, d.tree[key])
				}

			} else if d.tree[key] == nil {
				scope.Warnf("Ignoring unmatched resource %s", key)
				continue
			}
			node := d.tree[key]
			resources = resources.Insert(resp.Name)
			referenced := extractReferencedKeys(resp)
			for _, rkey := range referenced {
				child, f := d.getNode(rkey)
				if !f {
					requests[rkey.TypeUrl] = append(requests[rkey.TypeUrl], rkey.Name)
				}
				relate(node, child)
			}
		}
		removals := map[string][]string{}
		for _, resp := range msg.RemovedResources {
			key := ResourceKey{
				Name:    resp,
				TypeUrl: msg.TypeUrl,
			}
			if d.tree[key] == nil {
				scope.Warnf("Ignoring removing unmatched resource %s", key)
				continue
			}
			node := d.tree[key]
			d.deleteNode(node, removals)
		}

		d.mu.Lock()
		origLen := len(d.resources[msg.TypeUrl])
		newAdd := Union(d.resources[msg.TypeUrl], resources)
		addedLen := len(newAdd) - origLen
		removedLen := len(msg.RemovedResources)
		d.resources[msg.TypeUrl] = newAdd.Difference(sets.NewSet(msg.RemovedResources...))
		d.mu.Unlock()
		scope.WithLabels("type", msg.TypeUrl, "added", addedLen, "removed", removedLen, "removed refs", len(removals)).Debugf("got message")
		if dumpScope.DebugEnabled() {
			s, _ := (&jsonpb.Marshaler{}).MarshalToString(msg)
			dumpScope.Debug(s)
		}
		if dumpScope.InfoEnabled() {
			dumpScope.Info("\n" + d.dumpTree())
		}

		// TODO: Envoy does some smart "pausing" to allow the next push to come before we request
		for _, k := range keysOfMaps(requests, removals) {
			if len(requests[k]) == 0 && len(removals[k]) == 0 {
				continue
			}
			if err := d.send(&discovery.DeltaDiscoveryRequest{
				TypeUrl:                  k,
				ResourceNamesSubscribe:   requests[k],
				ResourceNamesUnsubscribe: removals[k],
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

func keysOfMaps(ms ...map[string][]string) []string {
	res := []string{}
	for _, m := range ms {
		for k := range m {
			res = append(res, k)
		}
	}
	// TODO sort in XDS ordering?
	sort.Strings(res)
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
			Name:    o.Name,
			TypeUrl: v3.EndpointType,
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
							Name:    r,
							TypeUrl: v3.RouteType,
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

// Istio on is broken
func Union(s, s2 sets.Set) sets.Set {
	result := sets.NewSet()
	for key := range s {
		result.Insert(key)
	}
	for key := range s2 {
		result.Insert(key)
	}
	return result
}

func (d *deltaClient) Watch() {
	scope.Infof("sending intial watches")
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
	return nil
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
			Parents:  map[*ResourceNode]struct{}{},
			Children: map[*ResourceNode]struct{}{},
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

func (d *deltaClient) deleteNode(node *ResourceNode, removals map[string][]string) {
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
