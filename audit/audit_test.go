package audit

import (
	"testing"

	"github.com/brickKit/be-sdk-go/v3/mergesafety"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChainOverDigests(t *testing.T) {
	a := chainNext("", "sha256:aa")
	assert.NotEqual(t, a, chainNext("", "sha256:bb"))
	assert.Equal(t, chainNext(a, "sha256:bb"), chainNext(chainNext("", "sha256:aa"), "sha256:bb"), "the same records, the same chain")
	assert.NotEqual(t, chainNext(chainNext("", "sha256:bb"), "sha256:aa"), chainNext(a, "sha256:bb"), "order matters")
}

// TestTheModuleIsMergeSafe: the module is compiled into the Go infra shell.
func TestTheModuleIsMergeSafe(t *testing.T) {
	findings, err := mergesafety.CheckDir(".")
	require.NoError(t, err)
	for _, f := range findings {
		t.Errorf("%v", f)
	}
}
