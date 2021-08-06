module github.com/howardjohn/pilot-load

go 1.16

require (
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cncf/udpa/go v0.0.0-20210322005330-6414d713912e
	github.com/envoyproxy/go-control-plane v0.9.10-0.20210708144103-3a95f2df6351
	github.com/felixge/fgprof v0.9.1
	github.com/ghodss/yaml v1.0.0
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.6
	github.com/lthibault/jitterbug v2.0.0+incompatible
	github.com/spf13/cobra v1.2.1
	go.uber.org/atomic v1.8.0 // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	google.golang.org/api v0.52.0
	google.golang.org/grpc v1.39.1
	google.golang.org/protobuf v1.27.1
	istio.io/api v0.0.0-20210806192855-2720268d182b
	istio.io/client-go v1.10.3
	istio.io/istio v0.0.0-20210806210018-d86a90873aa1
	istio.io/pkg v0.0.0-20210806192335-d58436bead18
	k8s.io/api v0.22.0
	k8s.io/apimachinery v0.22.0
	k8s.io/client-go v0.22.0
)

replace github.com/spf13/viper => github.com/istio/viper v1.3.3-0.20190515210538-2789fed3109c

// Old version had no license
replace github.com/chzyer/logex => github.com/chzyer/logex v1.1.11-0.20170329064859-445be9e134b2

// Avoid pulling in incompatible libraries
replace github.com/docker/distribution => github.com/docker/distribution v0.0.0-20191216044856-a8371794149d

replace github.com/docker/docker => github.com/moby/moby v17.12.0-ce-rc1.0.20200618181300-9dc6525e6118+incompatible

// Client-go does not handle different versions of mergo due to some breaking changes - use the matching version
replace github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
