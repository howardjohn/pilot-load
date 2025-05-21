package templates

import (
	"embed"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"

	"istio.io/istio/pkg/log"
)

//go:embed *.yaml
var FS embed.FS

func LoadBuiltin() map[string]*template.Template {
	entries, err := FS.ReadDir(".")
	if err != nil {
		log.Fatalf("failed to load templates: %v", err)
	}
	result := make(map[string]*template.Template)
	for _, entry := range entries {
		n := entry.Name()
		k := strings.TrimSuffix(n, filepath.Ext(n))
		by, err := FS.ReadFile(n)
		if err != nil {
			log.Fatalf("failed to load templates: %v", err)
		}
		tm, err := ParseTemplate(string(by))
		if err != nil {
			log.Fatalf("failed to load templates: %v", err)
		}
		result[k] = tm
	}
	return result
}

func ParseTemplate(s string) (*template.Template, error) {
	return template.New("test").Funcs(sprig.TxtFuncMap()).Parse(s)
}
