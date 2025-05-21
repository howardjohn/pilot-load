package reader

import (
	"bufio"
	"fmt"
	"io"
	"os"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	yamlserializer "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/yaml"

	"istio.io/istio/pkg/config/schema/collections"
	"istio.io/istio/pkg/config/schema/resource"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/log"
)

func ParseYamlFile(inputFile string) ([]controllers.Object, error) {
	f, err := os.Open(inputFile)
	if err != nil {
		return nil, err
	}
	return ParseYaml(f)
}

func ParseYaml(r io.Reader) ([]controllers.Object, error) {
	codecs := serializer.NewCodecFactory(kube.IstioScheme)
	deserializer := codecs.UniversalDeserializer()

	reader := yaml.NewYAMLReader(bufio.NewReader(r))
	resp := []controllers.Object{}
	for {
		chunk, err := reader.Read()
		if err == io.EOF {
			break
		}
		gvkp, err := yamlserializer.DefaultMetaFactory.Interpret(chunk)
		if err != nil {
			return nil, err
		}
		gvk := *gvkp
		var obj runtime.Object
		// We want to normalize types to a single version
		s, exists := collections.PilotGatewayAPI().FindByGroupVersionAliasesKind(resource.FromKubernetesGVK(&gvk))
		if exists {
			gvk = s.GroupVersionKind().Kubernetes()
			raw, err := kube.IstioScheme.New(s.GroupVersionKind().Kubernetes())
			if err != nil {
				return nil, err
			}
			if err := yaml.Unmarshal(chunk, &raw); err != nil {
				return nil, err
			}
			tm, err := apimeta.TypeAccessor(raw)
			if err != nil {
				return nil, err
			}
			tm.SetAPIVersion(s.APIVersion())
			obj = raw
		} else {
			log.Errorf("howardjohn: not exist... %v", gvk)
			obj, _, err = deserializer.Decode(chunk, &gvk, obj)
			if err != nil {
				return nil, fmt.Errorf("cannot parse message: %v", err)
			}
		}

		cobj := obj.(controllers.Object)
		log.Errorf("howardjohn: set gvk to %+v %T", gvk, cobj)
		cobj.GetObjectKind().SetGroupVersionKind(gvk)

		resp = append(resp, cobj)
	}

	return resp, nil
}
