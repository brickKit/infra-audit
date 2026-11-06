package audit

import (
	"encoding/json"
	"testing"

	protocol "github.com/brickKit/be-protocol/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The evidence store accepts exactly the categories of the protocol's evidence-record vectors (P12.9, assembly
// protocol 15's evidence table): a category the protocol adds (P12.23's evidence_double_failure) is not refused.
func TestTheCategoriesAreTheProtocols(t *testing.T) {
	raw, err := protocol.Vectors.ReadFile("vectors/evidence-record.json")
	require.NoError(t, err)
	var v struct {
		Cases []struct {
			ID       string `json:"id"`
			Expected struct {
				Categories []string `json:"categories"`
			} `json:"expected"`
		} `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &v))
	var want []string
	for _, c := range v.Cases {
		if c.ID == "categories" {
			want = c.Expected.Categories
		}
	}
	require.NotEmpty(t, want)
	assert.ElementsMatch(t, want, Categories)
}
