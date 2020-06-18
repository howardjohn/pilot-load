package kube

import (
	"context"
	"fmt"
	"time"

	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioscheme "istio.io/client-go/pkg/clientset/versioned/scheme"
	"istio.io/pkg/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
)

type Client struct {
	dynamic    dynamic.Interface
	kubernetes kubernetes.Interface
}

func NewClient(kubeconfig string) (*Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	// Gotta go fast
	config.QPS = 100
	config.Burst = 200
	if err != nil {
		return nil, err
	}
	d, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	k, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Client{
		dynamic:    d,
		kubernetes: k,
	}, nil
}

var deletePeriod int64 = 0

func (c *Client) Finalize(ns *v1.Namespace) error {
	_, err := c.kubernetes.CoreV1().Namespaces().Finalize(context.TODO(), ns, metav1.UpdateOptions{})
	return err
}

func (c *Client) Delete(o runtime.Object) error {
	us := toUnstructured(o)
	if us == nil {
		return fmt.Errorf("bad object %v", o)
	}
	gvr, kind := toGvr(o)
	cl := c.dynamic.Resource(gvr).Namespace(us.GetNamespace())
	us.SetGroupVersionKind(gvr.GroupVersion().WithKind(kind))
	return cl.Delete(context.TODO(), us.GetName(), metav1.DeleteOptions{GracePeriodSeconds: &deletePeriod})
}

func init() {
	if err := istioscheme.AddToScheme(scheme.Scheme); err != nil {
		panic(err.Error())
	}
}

// TODO make this generic
func toGvr(o runtime.Object) (schema.GroupVersionResource, string) {
	switch o.(type) {
	case *v1.Pod:
		return v1.SchemeGroupVersion.WithResource("pods"), "Pod"
	case *v1.Service:
		return v1.SchemeGroupVersion.WithResource("services"), "Service"
	case *v1.ServiceAccount:
		return v1.SchemeGroupVersion.WithResource("serviceaccounts"), "ServiceAccount"
	case *v1.Namespace:
		return v1.SchemeGroupVersion.WithResource("namespaces"), "Namespace"
	case *v1.Endpoints:
		return v1.SchemeGroupVersion.WithResource("endpoints"), "Endpoints"
	case *v1alpha3.VirtualService:
		return v1alpha3.SchemeGroupVersion.WithResource("virtualservices"), "VirtualService"
	case *v1alpha3.Sidecar:
		return v1alpha3.SchemeGroupVersion.WithResource("sidecars"), "Sidecar"
	case *v1alpha3.Gateway:
		return v1alpha3.SchemeGroupVersion.WithResource("gateways"), "Gateway"
	case *v1alpha3.DestinationRule:
		return v1alpha3.SchemeGroupVersion.WithResource("destinationrules"), "DestinationRule"
	default:
		panic(fmt.Sprintf("unsupported type %T", o))
	}
}

func (c *Client) Apply(o runtime.Object) error {
	us := toUnstructured(o)
	if us == nil {
		return fmt.Errorf("bad object %v", o)
	}
	gvr, kind := toGvr(o)
	backoff := wait.Backoff{Duration: time.Millisecond * 10, Factor: 2, Steps: 3}
	cl := c.dynamic.Resource(gvr).Namespace(us.GetNamespace())

	// TODO do we need to do this manually? We only need it for Istio types
	us.SetGroupVersionKind(gvr.GroupVersion().WithKind(kind))
	err := retry.RetryOnConflict(backoff, func() error {
		cur, err := cl.Get(context.TODO(), us.GetName(), metav1.GetOptions{})
		switch {
		case errors.IsNotFound(err):
			log.Debugf("creating resource: %s/%s/%s", us.GetKind(), us.GetName(), us.GetNamespace())
			_, err = cl.Create(context.TODO(), us, metav1.CreateOptions{})
			return err
		case err == nil:
			log.Debugf("updating resource: %s/%s/%s", us.GetKind(), us.GetName(), us.GetNamespace())
			us.SetResourceVersion(cur.GetResourceVersion())
			_, err := cl.Update(context.TODO(), us, metav1.UpdateOptions{})
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to apply %s/%s/%s: %v", us.GetKind(), us.GetName(), us.GetNamespace(), err)
	}
	return nil
}

func toUnstructured(o runtime.Object) *unstructured.Unstructured {
	unsObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(o)
	if err != nil {
		return nil
	}
	return &unstructured.Unstructured{Object: unsObj}
}
