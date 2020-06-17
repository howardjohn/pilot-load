package kube

import (
	"context"
	"fmt"
	"time"

	"istio.io/pkg/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
)

type Client struct {
	dynamic dynamic.Interface
}

func NewClient(kubeconfig string) (*Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	d, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Client{
		dynamic: d,
	}, nil
}

var deletePeriod int64 = 0

func (c *Client) Delete(o runtime.Object) error {
	us := toUnstructured(o)
	if us == nil {
		return fmt.Errorf("bad object %v", o)
	}
	cl := c.dynamic.Resource(toGvr(o)).Namespace(us.GetNamespace())
	return cl.Delete(context.TODO(), us.GetName(), metav1.DeleteOptions{GracePeriodSeconds: &deletePeriod})
}

// TODO make this generic
func toGvr(o runtime.Object) schema.GroupVersionResource {
	switch o.(type) {
	case *v1.Pod:
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	case *v1.Service:
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
	case *v1.ServiceAccount:
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}
	case *v1.Namespace:
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	case *v1.Endpoints:
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "endpoints"}
	default:
		panic(fmt.Sprintf("unsupported type %T", o))
	}
}

func (c *Client) Apply(o runtime.Object) error {
	us := toUnstructured(o)
	if us == nil {
		return fmt.Errorf("bad object %v", o)
	}
	backoff := wait.Backoff{Duration: time.Millisecond * 10, Factor: 2, Steps: 3}
	cl := c.dynamic.Resource(toGvr(o)).Namespace(us.GetNamespace())

	err := retry.RetryOnConflict(backoff, func() error {
		_, err := cl.Get(context.TODO(), us.GetName(), metav1.GetOptions{})
		switch {
		case errors.IsNotFound(err):
			log.Debugf("creating resource: %s/%s", us.GetName(), us.GetNamespace())
			_, err = cl.Create(context.TODO(), us, metav1.CreateOptions{})
			return err
		case err == nil:
			log.Debugf("updating resource: %s/%s", us.GetName(), us.GetNamespace())
			_, err := cl.Update(context.TODO(), us, metav1.UpdateOptions{})
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to apply %s/%s", us.GetName(), us.GetNamespace())
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
