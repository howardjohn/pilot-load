module github.com/howardjohn/file-based-istio

go 1.12

replace github.com/golang/glog => github.com/istio/glog v0.0.0-20190424172949-d7cfb6fa2ccd

replace k8s.io/klog => github.com/istio/klog v0.0.0-20190424230111-fb7481ea8bcf

replace github.com/spf13/viper => github.com/istio/viper v1.3.3-0.20190515210538-2789fed3109c

require (
	github.com/envoyproxy/go-control-plane v0.8.0
	github.com/gogo/protobuf v1.2.1
	google.golang.org/grpc v1.20.1
	istio.io/istio v0.0.0-20190525042921-d8a34e3aa93f
)
