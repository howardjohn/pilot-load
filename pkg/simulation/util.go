package simulation

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

func applyConfig(yaml string) error {
	c := exec.Command("kubectl", "apply", "-f", "-")
	c.Stdin = strings.NewReader(yaml)
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	return c.Run()
}

func deleteConfig(yaml string) error {
	log.Println("deleting config")
	c := exec.Command("kubectl", "delete", "-f", "-", "--force", "--grace-period=0", "--ignore-not-found", "--wait=false")
	c.Stdin = strings.NewReader(yaml)
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	return c.Run()
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
