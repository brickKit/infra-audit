package audit

import (
	"testing"

	"github.com/brickKit/be-sdk-go/v3/mergesafety"
	"github.com/stretchr/testify/require"
)

// TestTheModuleIsMergeSafe: the module is compiled into the Go infra shell: the entry package and every business
// module under internal/.
func TestTheModuleIsMergeSafe(t *testing.T) {
	findings, err := mergesafety.CheckDir(".")
	require.NoError(t, err)
	inner, err := mergesafety.CheckTree("../internal")
	require.NoError(t, err)
	findings = append(findings, inner...)
	for _, f := range findings {
		t.Errorf("%v", f)
	}
}
