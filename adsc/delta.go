package adsc

import (
	"fmt"
	"math"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/grpc"

	v3 "istio.io/istio/pilot/pkg/xds/v3"
)

type deltaClient struct {
	initialWatches []string
	node           *core.Node
	conn           *grpc.ClientConn
	client         discovery.AggregatedDiscoveryService_DeltaAggregatedResourcesClient
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
		scope.Debugf("got message for type %v", msg.TypeUrl)
		requests := map[string][]string{}
		for _, resp := range msg.Resources {
			switch msg.TypeUrl {
			case v3.ClusterType:
				o := &cluster.Cluster{}
				_ = resp.Resource.UnmarshalTo(o)
				switch v := o.ClusterDiscoveryType.(type) {
				case *cluster.Cluster_Type:
					if v.Type != cluster.Cluster_EDS {
						continue
					}
				}
				requests[v3.EndpointType] = append(requests[v3.EndpointType], resp.Name)
			case v3.ListenerType:
				o := &listener.Listener{}
				_ = resp.Resource.UnmarshalTo(o)
				for _, fc := range getFilterChains(o) {
					for _, f := range fc.GetFilters() {
						if f.GetTypedConfig().GetTypeUrl() == "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager" {
							hcm := &hcm.HttpConnectionManager{}
							_ = f.GetTypedConfig().UnmarshalTo(hcm)
							if r := hcm.GetRds().GetRouteConfigName(); r != "" {
								requests[v3.RouteType] = append(requests[v3.RouteType], r)
							}
						}
					}
				}
			}
		}

		for t, res := range requests {
			if err := d.send(&discovery.DeltaDiscoveryRequest{
				TypeUrl:                t,
				ResourceNamesSubscribe: res,
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
	scope.Debugf("send message for type %v (%v) for %v", dr.TypeUrl, reason, dr.ResourceNamesSubscribe)
	return d.client.Send(dr)
}
