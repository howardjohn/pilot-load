package client

import (
	"fmt"
	"io/ioutil"
	"path"
	"sync"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	hcm_filter "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	pilotv2 "istio.io/istio/pilot/pkg/proxy/envoy/v2"
	"istio.io/istio/pkg/adsc"
)

func makeADSC(addr string, client int) (*adsc.ADSC, error) {
	ip := fmt.Sprintf("127.0.%d.%d", client/256, client%256)
	return adsc.Dial(addr, "", &adsc.Config{
		IP: ip,
	})
}

func RunLoad(pilotAddress string, clients int) error {
	cons := make([]*adsc.ADSC, 0, clients)
	for cur := 0; cur < clients; cur++ {
		adsc, err := makeADSC(pilotAddress, cur)
		if err != nil {
			return err
		}
		cons = append(cons, adsc)
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	for _, con := range cons {
		con.Watch()
	}
	wg.Wait()
	return nil
}

func waitForAllConfig(adsc *adsc.ADSC) error {
	adsc.Watch()
	if _, err := adsc.Wait("cds", 10*time.Second); err != nil {
		return fmt.Errorf("failed to wait: %v", err)
	}
	if _, err := adsc.Wait("eds", 10*time.Second); err != nil {
		return fmt.Errorf("failed to wait: %v", err)
	}
	if _, err := adsc.Wait("lds", 10*time.Second); err != nil {
		return fmt.Errorf("failed to wait: %v", err)
	}
	if _, err := adsc.Wait("rds", 10*time.Second); err != nil {
		return fmt.Errorf("failed to wait: %v", err)
	}
	return nil
}

func WriteXDSConfig(adsc *adsc.ADSC, outdir string) error {
	if err := waitForAllConfig(adsc); err != nil {
		return err
	}

	clusters := []*v2.Cluster{}
	for _, c := range adsc.Clusters {
		clusters = append(clusters, c)
	}
	for _, c := range adsc.EDSClusters {
		clusters = append(clusters, c)
	}
	write(clusterResponse(clusters), outdir, "cds.yaml")

	listeners := []*v2.Listener{}
	for _, l := range adsc.HTTPListeners {
		listeners = append(listeners, l)
	}
	for _, l := range adsc.TCPListeners {
		listeners = append(listeners, l)
	}
	write(listenerResponse(listeners), outdir, "lds.yaml")

	for route, r := range adsc.Routes {
		write(routesResponse([]*v2.RouteConfiguration{r}), outdir, fmt.Sprintf("rds/%s.yaml", SanitizeName(route)))
	}

	for endpoint, e := range adsc.EDS {
		write(endpointsResponse([]*v2.ClusterLoadAssignment{e}), outdir, fmt.Sprintf("eds/%s.yaml", SanitizeName(endpoint)))
	}

	return nil
}

func write(r *v2.DiscoveryResponse, dir string, file string) {
	if dir == "" {
		fmt.Println(string(MarshallYaml(r)))
	} else {
		if err := ioutil.WriteFile(path.Join(dir, file), MarshallYaml(r), 0777); err != nil {
			panic(err)
		}
	}
}

func endpointsResponse(response []*v2.ClusterLoadAssignment) *v2.DiscoveryResponse {
	out := &v2.DiscoveryResponse{
		TypeUrl:     pilotv2.EndpointType,
		VersionInfo: "0",
	}

	for _, c := range response {
		cc, _ := types.MarshalAny(c)
		out.Resources = append(out.Resources, *cc)
	}

	return out
}

func clusterResponse(response []*v2.Cluster) *v2.DiscoveryResponse {
	out := &v2.DiscoveryResponse{
		TypeUrl:     pilotv2.ClusterType,
		VersionInfo: "0",
	}

	sanitizeClusterAds(response)

	for _, c := range response {
		cc, _ := types.MarshalAny(c)
		out.Resources = append(out.Resources, *cc)
	}

	return out
}

func listenerResponse(response []*v2.Listener) *v2.DiscoveryResponse {
	out := &v2.DiscoveryResponse{
		TypeUrl:     pilotv2.ListenerType,
		VersionInfo: "0",
	}

	sanitizeListenerAds(response)

	for _, c := range response {
		cc, _ := types.MarshalAny(c)
		out.Resources = append(out.Resources, *cc)
	}

	return out
}

func routesResponse(response []*v2.RouteConfiguration) *v2.DiscoveryResponse {
	out := &v2.DiscoveryResponse{
		TypeUrl:     pilotv2.RouteType,
		VersionInfo: "0",
	}

	for _, c := range response {
		cc, _ := types.MarshalAny(c)
		out.Resources = append(out.Resources, *cc)
	}

	return out
}

func sanitizeClusterAds(response []*v2.Cluster) {
	for _, r := range response {
		if r.EdsClusterConfig == nil {
			continue
		}
		path := fmt.Sprintf("/etc/config/eds/%s.yaml", SanitizeName(r.Name))
		r.EdsClusterConfig.EdsConfig = &core.ConfigSource{
			ConfigSourceSpecifier: &core.ConfigSource_Path{Path: path},
		}
	}
}

func sanitizeListenerAds(response []*v2.Listener) {
	for _, c := range response {
		for _, fc := range c.FilterChains {
			for _, f := range fc.Filters {
				switch f.Name {
				case "envoy.http_connection_manager":
					routeName := f.GetConfig().Fields["rds"].GetStructValue().GetFields()["route_config_name"].GetStringValue()
					if routeName == "" {
						continue
					}
					path := fmt.Sprintf("/etc/config/rds/%s.yaml", SanitizeName(routeName))
					rdsFileConfig, _ := util.MessageToStruct(&hcm_filter.Rds{
						RouteConfigName: routeName,
						ConfigSource: core.ConfigSource{
							ConfigSourceSpecifier: &core.ConfigSource_Path{Path: path},
						},
					})
					f.GetConfig().Fields["rds"] = &types.Value{
						Kind: &types.Value_StructValue{StructValue: rdsFileConfig},
					}
				default:
				}
			}
		}
	}
}
