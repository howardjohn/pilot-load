package kube

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"

	"istio.io/istio/pkg/cluster"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/kubeclient"
	kubetypes2 "istio.io/istio/pkg/config/schema/kubetypes"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kubetypes"
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/test/scopes"
)

type Client struct {
	kube.Client
	ClusterName string
}

func NewFakeClient(kf kube.Client) *Client {
	return &Client{
		ClusterName: "fake",
		Client:      kf,
	}
}

func NewClient(kubeconfig string, qps int) (*Client, error) {
	var clusterName string
	if _, err := os.Stat(kubeconfig); err == nil {
		loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}, nil)
		rc, err := loader.RawConfig()
		if err != nil {
			return nil, err
		}
		clusterName = rc.Contexts[rc.CurrentContext].Cluster
	} else {
		log.Infof("using in cluster kubeconfig")
	}
	rc, err := kube.DefaultRestConfig(kubeconfig, "", func(config *rest.Config) {
		config.QPS = float32(qps)
		config.Burst = qps * 2
	})
	if err != nil {
		return nil, err
	}

	kf, err := kube.NewClient(kube.NewClientConfigForRestConfig(rc), cluster.ID(clusterName))
	if err != nil {
		return nil, err
	}
	return &Client{
		ClusterName: clusterName,
		Client:      kf,
	}, nil
}

func (c *Client) Finalize(ns *v1.Namespace) error {
	scope.Debugf("finalizing namespace: %v", ns.Name)
	_, err := c.Kube().CoreV1().Namespaces().Finalize(context.TODO(), ns, metav1.UpdateOptions{})
	return err
}

var scope = log.RegisterScope("kube", "")

func ApplyRes[T controllers.Object](c *Client, o T) (T, error) {
	return internalApply(c, o, false)
}

func Apply[T controllers.Object](c *Client, o T) error {
	_, err := internalApply(c, o, false)
	return err
}

func ApplyFast[T controllers.Object](c *Client, o T) error {
	_, err := internalApply(c, o, true)
	return err
}

func ApplyRealSSA[T controllers.Object](c *Client, o T) error {
	name := o.GetName()
	ns := o.GetNamespace()
	cl := kubeclient.GetWriteClient[T](c, ns).(API[T])
	t := ptr.TypeName[T]()

	buf := &bytes.Buffer{}
	if err := kube.IstioCodec.LegacyCodec(kubetypes2.MustGVRFromType[T]().GroupVersion()).Encode(o, buf); err != nil {
		return err
	}
	b := buf.Bytes()
	opts := metav1.PatchOptions{
		Force:        ptr.Of(true),
		FieldManager: "pilot-load",
	}
	_, err := cl.Patch(context.TODO(), name, types.ApplyPatchType, b, opts)
	if err != nil {
		return fmt.Errorf("failed to ssa %s/%s/%s: %v", t, name, ns, err)
	}
	scope.Debugf("fast ssa resource: %s/%s/%s", t, name, ns)
	if hasStatus(c, o) {
		scope.Debugf("fast ssa resource status: %s/%s.%s", t, name, ns)
		if _, err := cl.Patch(context.TODO(), name, types.ApplyPatchType, b, opts, "status"); err != nil {
			return fmt.Errorf("ssa status %s/%s/%s: %v", t, name, ns, err)
		}
	}
	return nil
}

func ApplyStatus[T controllers.Object](c *Client, o T) error {
	name := o.GetName()
	ns := o.GetNamespace()
	cl := kubeclient.GetWriteClient[T](c, ns).(API[T])
	t := ptr.TypeName[T]()
	scope.Debugf("fast updating resource status: %s/%s.%s", t, name, ns)
	if _, err := cl.(kubetypes.WriteStatusAPI[T]).UpdateStatus(context.TODO(), o, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update status: %v", err)
	}
	return nil
}

// TypeIsConcrete checks if T is an interface or a concrete type
func TypeIsConcrete[T any]() bool {
	et := ptr.Empty[T]()
	return reflect.TypeOf(et) != nil
}

// Create creates a new object, and returns true if it was newly created.
// If it already exists, no action is taken and false is returned
// Status is not written
func Create[T controllers.Object](c *Client, o T) (bool, error) {
	if !TypeIsConcrete[T]() {
		cl, gvr := dynamicClient(c, o)
		scope.Debugf("creating resource: %s/%s/%s", gvr, o.GetName(), o.GetNamespace())
		o := toUnstructured(o)
		if _, err := cl.Create(context.Background(), o, metav1.CreateOptions{}); err != nil {
			if errors.IsAlreadyExists(err) {
				scope.Debugf("skipped resource, already exists: %s/%s/%s", gvr, o.GetName(), o.GetNamespace())
				return false, nil
			}
			if errors.IsForbidden(err) && strings.Contains(err.Error(), "exceeded quota") {
				scope.Warnf("skipped resource, exceeded quota: %s/%s/%s", gvr, o.GetName(), o.GetNamespace())
				return false, nil
			}
			return false, fmt.Errorf("create resource: %v", err)
		}
		return true, nil
	}
	cl := kubeclient.GetWriteClient[T](c, o.GetNamespace()).(API[T])

	t := ptr.TypeName[T]()
	scope.Debugf("creating resource: %s/%s/%s", t, o.GetName(), o.GetNamespace())
	if _, err := cl.Create(context.Background(), o, metav1.CreateOptions{}); err != nil {
		if errors.IsAlreadyExists(err) {
			scope.Debugf("skipped resource, already exists: %s/%s/%s", t, o.GetName(), o.GetNamespace())
			return false, nil
		}
		if errors.IsForbidden(err) && strings.Contains(err.Error(), "exceeded quota") {
			scope.Warnf("skipped resource, exceeded quota: %s/%s/%s", t, o.GetName(), o.GetNamespace())
			return false, nil
		}
		return false, fmt.Errorf("create resource: %v", err)
	}
	return true, nil
}

func dynamicClient[T controllers.Object](c *Client, o T) (dynamic.ResourceInterface, schema.GroupVersionResource) {
	gvr := toGvr[T](o)
	raw := c.Dynamic().Resource(gvr)
	var cl dynamic.ResourceInterface = raw
	if o.GetNamespace() != "" {
		cl = raw.Namespace(o.GetNamespace())
	}
	return cl, gvr
}

func toGvr[T controllers.Object](o T) schema.GroupVersionResource {
	kk := o.GetObjectKind().GroupVersionKind()
	ik := config.GroupVersionKind{
		Group:   kk.Group,
		Version: kk.Version,
		Kind:    kk.Kind,
	}
	gvr := gvk.MustToGVR(ik)
	return gvr
}

func Delete[T controllers.Object](c *Client, o T) error {
	if !TypeIsConcrete[T]() {
		cl, gvr := dynamicClient(c, o)
		scope.Debugf("deleting resource: %s/%s/%s", gvr, o.GetName(), o.GetNamespace())
		if err := cl.Delete(context.Background(), o.GetName(), metav1.DeleteOptions{GracePeriodSeconds: ptr.Of(int64(0))}); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
		return nil
	}
	cl := kubeclient.GetWriteClient[T](c, o.GetNamespace()).(API[T])
	scope.Debugf("deleting resource: %s/%s/%s", ptr.TypeName[T](), o.GetName(), o.GetNamespace())
	if err := cl.Delete(context.Background(), o.GetName(), metav1.DeleteOptions{GracePeriodSeconds: ptr.Of(int64(0))}); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

type API[T runtime.Object] interface {
	kubetypes.WriteAPI[T]
	Get(ctx context.Context, name string, opts metav1.GetOptions) (T, error)
}

func internalApply[T controllers.Object](c *Client, o T, skipGet bool) (T, error) {
	empty := ptr.Empty[T]()
	name := o.GetName()
	ns := o.GetNamespace()
	cl := kubeclient.GetWriteClient[T](c, ns).(API[T])
	backoff := wait.Backoff{Duration: time.Millisecond * 10, Factor: 2, Steps: 3}
	t := ptr.TypeName[T]()

	if skipGet {
		var res T
		err := retry.RetryOnConflict(backoff, func() error {
			scope.Debugf("fast creating resource: %s/%s/%s", t, name, ns)
			var err error
			res, err = cl.Create(context.TODO(), o, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("create: %v", err)
			}
			if hasStatus(c, o) {
				scope.Debugf("fast updating resource status: %s/%s.%s", t, name, ns)
				if _, err := cl.(kubetypes.WriteStatusAPI[T]).UpdateStatus(context.TODO(), o, metav1.UpdateOptions{}); err != nil {
					return fmt.Errorf("update status: %v", err)
				}
			}
			return nil
		})
		if err != nil {
			return empty, fmt.Errorf("failed to create %s/%s/%s: %v", t, name, ns, err)
		}
		return res, nil
	}

	res, err := cl.Get(context.TODO(), name, metav1.GetOptions{})
	switch {
	// New resource to create. Create and maybe updat status
	case errors.IsNotFound(err):
		scope.Debugf("creating resource: %s/%s/%s", t, name, ns)
		if res, err = cl.Create(context.TODO(), o, metav1.CreateOptions{}); err != nil {
			return empty, fmt.Errorf("create: %w", err)
		}
		o.SetResourceVersion(res.GetResourceVersion())
		if hasStatus(c, o) {
			if err := updateStatus[T](cl, o); err != nil {
				return empty, err
			}
		}
		return res, nil
		// Existing resource. Update then UpdateStatus
	default:
		o.SetResourceVersion(res.GetResourceVersion())
		res, err := update[T](cl, o)
		if err != nil {
			return empty, err
		}
		o.SetResourceVersion(res.GetResourceVersion())
		if hasStatus(c, o) {
			if err := updateStatus[T](cl, o); err != nil {
				return res, err
			}
		}
		return res, nil
	}
}

func updateStatus[T controllers.Object](cl API[T], o T) error {
	backoff := wait.Backoff{Duration: time.Millisecond * 10, Factor: 2, Steps: 3}
	scope.Debugf("updating resource status: %s/%s.%s", ptr.TypeName[T](), o.GetName(), o.GetNamespace())

	firstRun := true
	err := retry.RetryOnConflict(backoff, func() error {
		if firstRun {
			firstRun = false
		} else {
			current, err := cl.Get(context.TODO(), o.GetName(), metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("get: %w", err)
			}
			o.SetResourceVersion(current.GetResourceVersion())
		}
		if _, err := cl.(kubetypes.WriteStatusAPI[T]).UpdateStatus(context.TODO(), o, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func update[T controllers.Object](cl API[T], o T) (T, error) {
	backoff := wait.Backoff{Duration: time.Millisecond * 10, Factor: 2, Steps: 3}
	scope.Debugf("updating resource: %s/%s.%s", ptr.TypeName[T](), o.GetName(), o.GetNamespace())

	firstRun := true
	var res T
	err := retry.RetryOnConflict(backoff, func() error {
		if firstRun {
			firstRun = false
		} else {
			current, err := cl.Get(context.TODO(), o.GetName(), metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("get: %w", err)
			}
			o.SetResourceVersion(current.GetResourceVersion())
		}
		var err error
		if res, err = cl.(kubetypes.WriteAPI[T]).Update(context.TODO(), o, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update: %w", err)
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	return res, nil
}

func hasStatus[T controllers.Object](c *Client, o T) bool {
	if _, ok := c.Kube().(*kubefake.Clientset); ok {
		// Fake client has no status...
		return false
	}
	return hasStatusInternal[T](o)
}

func hasStatusInternal[T controllers.Object](o T) bool {
	v := reflect.ValueOf(o).Elem().FieldByName("Status")
	if v == (reflect.Value{}) {
		return false
	}
	return !v.IsZero()
}

func (c *Client) FetchRootCert() (string, error) {
	cm, err := c.Kube().CoreV1().ConfigMaps("istio-system").Get(context.TODO(), "istio-ca-root-cert", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return cm.Data["root-cert.pem"], nil
}

// 7 days
var saTokenExpiration int64 = 60 * 60 * 24 * 7

func (c *Client) CreateServiceAccountToken(aud, ns, serviceAccount string) (string, time.Time, error) {
	scopes.Framework.Debugf("Creating service account token for: %s/%s", ns, serviceAccount)

	token, err := c.Kube().CoreV1().ServiceAccounts(ns).CreateToken(context.TODO(), serviceAccount,
		&authenticationv1.TokenRequest{
			Spec: authenticationv1.TokenRequestSpec{
				Audiences:         []string{aud},
				ExpirationSeconds: &saTokenExpiration,
			},
		}, metav1.CreateOptions{})
	if err != nil {
		return "", time.Time{}, err
	}
	return token.Status.Token, token.Status.ExpirationTimestamp.Time, nil
}

func toUnstructured(o runtime.Object) *unstructured.Unstructured {
	unsObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(o)
	if err != nil {
		return nil
	}
	return &unstructured.Unstructured{Object: unsObj}
}
