package config

import (
	"testing"
	"text/template"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

func TestNewTemplatedCopies(t *testing.T) {
	tmpl := template.Must(template.New("t").Parse(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{.Name}}-{{.Index}}
`))
	setDefaults := func(cfg map[string]any) {
		cfg[Namespace] = "ns"
		cfg[Name] = "app"
	}
	names := func(sims []*Templated) []string {
		res := []string{}
		for _, s := range sims {
			objs, err := s.getTemplated()
			if err != nil {
				t.Fatal(err)
			}
			for _, o := range objs {
				res = append(res, o.GetNamespace()+"/"+o.GetName())
			}
		}
		return res
	}

	for _, count := range []int{0, 1} {
		sims := NewTemplatedCopies(model.ConfigTemplate{Name: "t", Count: count}, tmpl, setDefaults)
		if got := names(sims); len(got) != 1 || got[0] != "ns/app-0" {
			t.Fatalf("count=%d: got %v", count, got)
		}
	}

	base := map[string]any{"foo": "bar"}
	sims := NewTemplatedCopies(model.ConfigTemplate{Name: "t", Count: 3, Config: base}, tmpl, setDefaults)
	got := names(sims)
	want := []string{"ns/app-0", "ns/app-1", "ns/app-2"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	// Each copy must have its own config map, and the input must not be mutated
	sims[0].Spec.Config["foo"] = "changed"
	if sims[1].Spec.Config["foo"] != "bar" {
		t.Fatalf("configs are shared between copies")
	}
	if len(base) != 1 || base["foo"] != "bar" {
		t.Fatalf("input config mutated: %v", base)
	}
}
