//go:build integration

package itest

import (
	"context"
	"time"

	sdkaudit "github.com/brickKit/be-sdk-go/v3/audit"
	"github.com/brickKit/be-sdk-go/v3/canon"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/records"
	core "github.com/brickKit/infra-audit/v3/internal/core"
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
)

// receive: three records received in order, storage sequences 1-3, the chain over their digests; the same again
// answers the original receipts.
func (fx *fixture) receive() {
	t := fx.t
	// three records: received in order, storage sequences 1-3, the chain over their digests
	batch := []sdkaudit.Wire{wire(t, 1, "demo.order.create"), wire(t, 2, "demo.order.confirm"), wire(t, 3, "demo.order.ship")}
	rs := fx.send(batch...)
	chain := ""
	for i, r := range rs {
		assert.Equal(t, "received", r.Status)
		assert.Equal(t, batch[i].ID, r.ID)
		assert.Equal(t, int64(i+1), r.Seq)
		chain = core.ChainNext(chain, batch[i].Digest)
		assert.Equal(t, chain, r.Chain)
	}
	again := fx.send(batch...)
	assert.Equal(t, rs, again, "stored before: the original receipts")
	fx.batch = batch
}

// refusals: another digest under a stored ID, bytes that are not the record, an ID that is not the record's; a gap in
// the source's sequence is reported and the record still stored; nothing is backed up yet, so nothing may be deleted.
func (fx *fixture) refusals() {
	t, ctx := fx.t, fx.ctx
	// another digest under a stored ID; bytes that are not the record; an ID that is not the record's
	raw, digest, err := records.Seal("be.audit.record", map[string]any{"id": fx.batch[0].ID, "occurred_at": canon.Time(time.Now()),
		"category": "business_change", "operation": "demo.order.cancel", "outcome": "succeeded"})
	require.NoError(t, err)
	other := sdkaudit.Wire{ID: fx.batch[0].ID, Seq: 4, Epoch: "1.1", Digest: digest, Record: raw}
	broken := wire(t, 4, "demo.order.cancel")
	broken.Digest = fx.batch[1].Digest
	misnamed := wire(t, 4, "demo.order.cancel")
	misnamed.ID = ids.New().String()
	rs := fx.send(other, broken, misnamed)
	assert.Equal(t, []string{"conflict", "invalid", "invalid"}, []string{rs[0].Status, rs[1].Status, rs[2].Status})

	// a gap in the source's sequence is reported and the record still stored
	gap := wire(t, 6, "demo.order.close")
	assert.Equal(t, "received", fx.send(gap)[0].Status)
	assert.Contains(t, fx.logs.String(), "a gap in a source's audit records")

	// nothing is backed up yet: nothing may be deleted
	wm, err := fx.sender.Watermark(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), wm.Seq)
	assert.False(t, wm.BackedUpInEpoch)
}

// restart: the audit component stops and comes back; the source sends what it held, the earlier records keep their
// receipts.
func (fx *fixture) restart() {
	t, ctx := fx.t, fx.ctx
	var rs []sdkaudit.Receipt
	// the audit component stops and comes back: the source sends what it held, and the earlier records stay as they were
	fx.stop()
	sctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	_, err := fx.sender.Send(sctx, []sdkaudit.Wire{wire(t, 7, "demo.order.archive")})
	cancel()
	assert.Error(t, err, "the audit component is down")
	fx.stop = fx.start()
	held := wire(t, 7, "demo.order.archive")
	require.Eventually(t, func() bool { // the source's transfer job retries every 5 s; the channel reconnects
		sctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		rs, err = fx.sender.Send(sctx, []sdkaudit.Wire{held, fx.batch[0]})
		return err == nil
	}, 15*time.Second, 200*time.Millisecond)
	assert.Equal(t, "received", rs[0].Status)
	assert.Equal(t, int64(5), rs[0].Seq, "the storage sequence continues")
	assert.Equal(t, int64(1), rs[1].Seq, "an earlier record keeps its receipt")
}
