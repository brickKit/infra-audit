package audit

import (
	"os"
	"testing"

	"github.com/brickKit/be-sdk-go/v3/launcher"
)

// ownKeys are the descriptions of the component's own keys (and of the SDK keys it describes its own way).
var ownKeys = map[string]string{
	"AUDIT_EVIDENCE_READERS": "the components that may list the evidence and the audit records (comma-separated component IDs); default ops/recover, the recovery tool",
	"AUDIT_GRPC_URL":         "process-level; for this component its own system surface ($endpoint:infra/audit:grpc): its own audit records go through the same intake",
}

// TestTheManifestDeclaresWhatTheSDKReads: component.yaml's configSchema is exactly the keys the SDK reads (3.0.17 was
// released without RECOVERY_PHASE: this component had no such check).
func TestTheManifestDeclaresWhatTheSDKReads(t *testing.T) {
	raw, err := os.ReadFile("../component.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if p := launcher.ManifestProblems(raw, Definition(), ownKeys); len(p) > 0 {
		t.Errorf("component.yaml's configSchema differs from what the SDK reads:\n%v\nthe block:\n%s", p,
			launcher.ConfigSchemaYAML(Definition(), ownKeys))
	}
}
