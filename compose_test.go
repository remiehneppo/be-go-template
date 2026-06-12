package be_go_template

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestDockerComposeDeclaresCoreServices(t *testing.T) {
	cmd := exec.Command("docker", "compose", "config", "--services")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose config --services error = %v output = %s", err, string(out))
	}
	services := strings.Fields(string(bytes.TrimSpace(out)))
	want := map[string]bool{"mongo": false, "redis": false, "api": false}
	for _, service := range services {
		if _, ok := want[service]; ok {
			want[service] = true
		}
	}
	for service, found := range want {
		if !found {
			t.Fatalf("service %q missing from compose output: %v", service, services)
		}
	}
}
