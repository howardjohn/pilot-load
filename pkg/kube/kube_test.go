package kube

import (
	"testing"

	v1 "k8s.io/api/core/v1"

	"istio.io/api/meta/v1alpha1"
	networkingclientv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
)

func TestCreate(t *testing.T) {
	c := NewFakeClient(kube.NewFakeClient())
	t.Run("typed", func(t *testing.T) {
		Create(c, &v1.Pod{})
	})
	t.Run("untyped", func(t *testing.T) {
		var untyped controllers.Object = &v1.Pod{}
		untyped.GetObjectKind().SetGroupVersionKind(gvk.Pod.Kubernetes())
		Create(c, untyped)
	})
}

func TestGetStatus(t *testing.T) {
	type testCase struct {
		name string
		i    controllers.Object
		want bool
	}
	tests := []testCase{
		{
			name: "pod no status",
			i:    &v1.Pod{},
			want: false,
		},
		{
			name: "istio no status",
			i:    &networkingclientv1alpha3.VirtualService{},
			want: false,
		},
		{
			name: "pod status",
			i:    &v1.Pod{Status: v1.PodStatus{PodIP: "1"}},
			want: true,
		},
		{
			name: "istio status",
			i:    &networkingclientv1alpha3.VirtualService{Status: v1alpha1.IstioStatus{ObservedGeneration: 2}},
			want: true,
		},
		{
			name: "no field",
			i:    &v1.Endpoints{},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasStatusInternal(tt.i); got != tt.want {
				t.Errorf("hasStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}
