package audit

import (
	"testing"

	"github.com/brickKit/be-sdk-go/v3/mergesafety"
	"github.com/stretchr/testify/require"
)

// TestTheModuleIsMergeSafe: the module is compiled into the Go infra shell.
func TestTheModuleIsMergeSafe(t *testing.T) {
	findings, err := mergesafety.CheckDir(".")
	require.NoError(t, err)
	for _, f := range findings {
		t.Errorf("%v", f)
	}
}
