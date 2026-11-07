//go:build integration

package itest

import (
	"time"

	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
)

// chainCheck: AU14: the chain check finds a record changed in place, the last one taken off the end (only the
// source's head tells), the first one taken out, one taken out of the middle.
func (fx *fixture) chainCheck() {
	t, ctx := fx.t, fx.ctx
	// AU14: the chain check finds a record changed in place and one taken out of the middle
	breaks := func() (n int) {
		require.NoError(t, fx.admin.QueryRow(ctx, "SELECT count(*) FROM "+itSchema+".audit_chain_break").Scan(&n))
		return n
	}
	time.Sleep(2500 * time.Millisecond)
	require.Zero(t, breaks(), "the precondition: every chain verifies")
	_, err := fx.admin.Exec(ctx, "UPDATE "+itSchema+".audit_record SET record = $2 WHERE id = $1", fx.batch[1].ID, fx.batch[2].Record)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return breaks() == 1 }, 10*time.Second, 200*time.Millisecond, "a changed record")
	assert.Contains(t, fx.logs.String(), `"event":"audit_chain_broken"`)
	_, err = fx.admin.Exec(ctx, "UPDATE "+itSchema+".audit_record SET record = $2 WHERE id = $1", fx.batch[1].ID, fx.batch[1].Record)
	require.NoError(t, err)
	_, err = fx.admin.Exec(ctx, "DELETE FROM "+itSchema+".audit_chain_break")
	require.NoError(t, err)
	time.Sleep(2500 * time.Millisecond)
	require.Zero(t, breaks(), "put back: the chain verifies again")
	// the last record taken off the end: only the source's head tells
	var lastID string
	require.NoError(t, fx.admin.QueryRow(ctx, "SELECT id::text FROM "+itSchema+".audit_record WHERE source = 'demo/orders' "+
		"ORDER BY storage_seq DESC LIMIT 1").Scan(&lastID))
	for _, q := range []string{"CREATE TABLE " + itSchema + ".au14_kept AS SELECT * FROM " + itSchema + ".audit_record WHERE id = $1",
		"DELETE FROM " + itSchema + ".audit_record WHERE id = $1"} {
		_, err = fx.admin.Exec(ctx, q, lastID)
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool { return breaks() == 1 }, 10*time.Second, 200*time.Millisecond, "a record taken off the end")
	var detail string
	require.NoError(t, fx.admin.QueryRow(ctx, "SELECT detail FROM "+itSchema+".audit_chain_break").Scan(&detail))
	assert.Contains(t, detail, "the head")
	for _, q := range []string{"INSERT INTO " + itSchema + ".audit_record SELECT * FROM " + itSchema + ".au14_kept",
		"DELETE FROM " + itSchema + ".audit_chain_break"} {
		_, err = fx.admin.Exec(ctx, q)
		require.NoError(t, err)
	}
	time.Sleep(2500 * time.Millisecond)
	require.Zero(t, breaks(), "put back: the chain verifies again")

	// the first record taken out (nothing of the source was ever archived): the chain starts there, so the break is
	// at the record after it (A158: only records that left by a release let the chain start mid-way)
	var firstID string
	require.NoError(t, fx.admin.QueryRow(ctx, "SELECT id::text FROM "+itSchema+".audit_record WHERE source = 'demo/orders' "+
		"ORDER BY storage_seq LIMIT 1").Scan(&firstID))
	for _, q := range []string{"CREATE TABLE " + itSchema + ".au14_first AS SELECT * FROM " + itSchema + ".audit_record WHERE id = $1",
		"DELETE FROM " + itSchema + ".audit_record WHERE id = $1"} {
		_, err = fx.admin.Exec(ctx, q, firstID)
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool { return breaks() == 1 }, 10*time.Second, 200*time.Millisecond, "the first record taken out")
	for _, q := range []string{"INSERT INTO " + itSchema + ".audit_record SELECT * FROM " + itSchema + ".au14_first",
		"DELETE FROM " + itSchema + ".audit_chain_break"} {
		_, err = fx.admin.Exec(ctx, q)
		require.NoError(t, err)
	}
	time.Sleep(2500 * time.Millisecond)
	require.Zero(t, breaks(), "put back: the chain verifies again")

	// one taken out of the middle: the break is at the record after it
	_, err = fx.admin.Exec(ctx, "DELETE FROM "+itSchema+".audit_record WHERE id = $1", fx.batch[1].ID)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return breaks() == 1 }, 10*time.Second, 200*time.Millisecond, "a record taken out")
	var at int64
	require.NoError(t, fx.admin.QueryRow(ctx, "SELECT at_seq FROM "+itSchema+".audit_chain_break").Scan(&at))
	assert.Equal(t, int64(3), at, "the record after the one taken out")
}
