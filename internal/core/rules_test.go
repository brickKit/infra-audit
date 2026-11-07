package core

import (
	"testing"

	assert "github.com/stretchr/testify/assert"
)

func TestChainOverDigests(t *testing.T) {
	a := ChainNext("", "sha256:aa")
	assert.NotEqual(t, a, ChainNext("", "sha256:bb"))
	assert.Equal(t, ChainNext(a, "sha256:bb"), ChainNext(ChainNext("", "sha256:aa"), "sha256:bb"), "the same records, the same chain")
	assert.NotEqual(t, ChainNext(ChainNext("", "sha256:bb"), "sha256:aa"), ChainNext(a, "sha256:bb"), "order matters")
}
