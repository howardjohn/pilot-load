package simulation

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v2"
)

type KubernetesObject struct {
	Kind     string
	Metadata map[string]interface{}
}

func logYaml(prefix, y string) {
	for _, p := range strings.Split(y, "---") {
		o := KubernetesObject{}
		if err := yaml.Unmarshal([]byte(p), &o); err != nil {
			log.Println("Failed to unmarshal with error ", err, p)
		}
		log.Println(fmt.Sprintf("%s: %s/%s.%v", prefix, o.Kind, o.Metadata["name"], o.Metadata["namespace"]))
	}
}

func applyConfig(yaml string) error {
	c := exec.Command("kubectl", "apply", "-f", "-")
	c.Stdin = strings.NewReader(yaml)
	c.Stderr = os.Stderr
	//c.Stdout = os.Stdout
	//logYaml("apply", yaml)
	if err := c.Run(); err != nil {
		return fmt.Errorf("kubectl apply: %v", err)
	}
	return nil
}

func deleteConfig(yaml string) error {
	c := exec.Command("kubectl", "delete", "-f", "-", "--force", "--grace-period=0", "--ignore-not-found", "--wait=false")
	c.Stdin = strings.NewReader(yaml)
	c.Stderr = os.Stderr
	//c.Stdout = os.Stdout
	//logYaml("delete", yaml)
	if err := c.Run(); err != nil {
		return fmt.Errorf("kubectl delete: %v", err)
	}
	return nil
}

func deleteNamespace(name string) error {
	log.Println("deleting namespace ", name)
	json := `{"kind":"Namespace","spec":{"finalizers":[]},"apiVersion":"v1","metadata":{"name":"%s"}}`
	c := exec.Command("kubectl", "replace", "-f", "-", "--raw", fmt.Sprintf("/api/v1/namespaces/%s/finalize", name))
	c.Stdin = strings.NewReader(fmt.Sprintf(json, name))
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	return c.Run()
}

func AddError(e1, e2 error) error {
	if e1 == nil {
		return e2
	}
	if e2 == nil {
		return e1
	}
	return fmt.Errorf("%v and %v", e1, e2)
}
