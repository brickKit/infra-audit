package audit

import (
	"context"

	core "github.com/brickKit/infra-audit/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/recovery"
)

// RecoveryChecks registers what a full recovery asks this member (be-protocol chapter 14). The audit store never goes
// back with the others (the recovery tool leaves its schema out), so whatever its own audit names is still there.
func (m *Module) RecoveryChecks(c *recovery.Checks) error {
	c.Presence(core.Records, func(_ context.Context, _ db.Tx, ids []string) (map[string]int64, error) {
		out := make(map[string]int64, len(ids))
		for _, id := range ids {
			out[id] = 0
		}
		return out, nil
	})
	return nil
}
