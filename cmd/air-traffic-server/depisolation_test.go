package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The control-plane binary must stay stdlib-only and must never link a
// gateway package: the gateway is allowed dependencies the control plane is
// forbidden from inheriting (inference-gateway-build-plan.md, G0).
func TestServerClosureExcludesGatewayAndThirdParty(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/jchigg2000-git/air-traffic/cmd/air-traffic-server").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasPrefix(dep, "github.com/jchigg2000-git/air-traffic/internal/gateway") {
			t.Errorf("control-plane closure links gateway package %s", dep)
		}
		if strings.HasPrefix(dep, "github.com/jchigg2000-git/air-traffic/") || strings.HasPrefix(dep, "vendor/") {
			continue // own module, or std-vendored
		}
		if first, _, _ := strings.Cut(dep, "/"); strings.Contains(first, ".") {
			t.Errorf("control-plane closure links third-party module %s", dep)
		}
	}
}
