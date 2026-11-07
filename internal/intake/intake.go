package intake

import (
	"context"
	"errors"

	archive "github.com/brickKit/infra-audit/v3/internal/archive"
	core "github.com/brickKit/infra-audit/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/grpcx"
	"github.com/brickKit/be-sdk-go/v3/records"
	auditv3 "github.com/brickKit/infra-audit/contract/v3/gen/go/be/audit/v3"
	"github.com/jackc/pgx/v5"
)

// Submit is IntakeService.Submit (contract/README.md, "审计组件的义务"): every item checked, stored once per source
// and ID under the storage counter's lock, a receipt for each in order; the answer leaves only after the commit.
func Submit(m *core.Base, ctx context.Context, in *auditv3.SubmitRequest) (*auditv3.SubmitResponse, error) {
	source, _ := grpcx.Caller(ctx)
	if len(in.GetItems()) > core.MaxBatch {
		return nil, errs.New(errs.ReasonBatchTooLarge, "at most 500 records a call").GRPCStatus().Err()
	}
	archived, err := archive.Archived(m, ctx, source, in.GetItems()) // before the intake's transaction: no store read inside it
	if err != nil {
		return nil, err
	}
	out := &auditv3.SubmitResponse{}
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		out.Receipts = out.Receipts[:0]
		epoch, err := core.EpochOf(ctx, tx)
		if err != nil {
			return err
		}
		var counterEpoch *string
		var last int64
		if err := tx.QueryRow(ctx, "SELECT epoch, last_seq FROM audit_counter FOR UPDATE").Scan(&counterEpoch, &last); err != nil {
			return err
		}
		if counterEpoch != nil && *counterEpoch != epoch { // a new epoch: the old one's fork point goes into the history
			if _, err := tx.Exec(ctx, "INSERT INTO audit_epoch (epoch, fork) VALUES ($1, $2) ON CONFLICT DO NOTHING",
				*counterEpoch, last); err != nil {
				return err
			}
			last = 0
		}
		var chain, srcEpoch string
		var srcLast int64
		err = tx.QueryRow(ctx, "SELECT chain, source_epoch, last_seq FROM audit_source WHERE source = $1 FOR UPDATE", source).
			Scan(&chain, &srcEpoch, &srcLast)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		for _, it := range in.GetItems() {
			r, err := item(m, ctx, tx, source, epoch, &last, &chain, &srcEpoch, &srcLast, it, archived[it.GetId()])
			if err != nil {
				return err
			}
			out.Receipts = append(out.Receipts, r)
		}
		if _, err := tx.Exec(ctx, "UPDATE audit_counter SET epoch = $1, last_seq = $2", epoch, last); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO audit_source (source, chain, source_epoch, last_seq) VALUES ($1, $2, $3, $4)
			ON CONFLICT (source) DO UPDATE SET chain = EXCLUDED.chain, source_epoch = EXCLUDED.source_epoch,
				last_seq = EXCLUDED.last_seq`, source, chain, srcEpoch, srcLast)
		return err
	})
	return out, err
}

// item stores one record or explains why not.
func item(m *core.Base, ctx context.Context, tx db.Tx, source, epoch string, last *int64, chain, srcEpoch *string,
	srcLast *int64, it *auditv3.Item, arch *archive.StoredRecord) (*auditv3.Receipt, error) {
	rec, err := records.Open("be.audit.record", it.GetRecord(), it.GetDigest())
	if err != nil || rec["id"] != it.GetId() {
		m.RT.Logger.Error("an audit record was refused as invalid (integrity event)", "source", source, "id", it.GetId())
		return &auditv3.Receipt{Id: it.GetId(), Status: auditv3.ReceiptStatus_RECEIPT_STATUS_INVALID}, nil
	}
	var digest, recEpoch, recChain string
	var seq int64
	err = tx.QueryRow(ctx, "SELECT digest, epoch, storage_seq, chain FROM audit_record WHERE source = $1 AND id = $2",
		source, it.GetId()).Scan(&digest, &recEpoch, &seq, &recChain)
	if errors.Is(err, pgx.ErrNoRows) && arch != nil { // stored before and archived since: its archived copy answers
		digest, recEpoch, seq, recChain, err = arch.Digest, arch.Epoch, arch.Seq, arch.Chain, nil
	}
	switch {
	case err == nil && digest == it.GetDigest(): // stored before: the original receipt
		return &auditv3.Receipt{Id: it.GetId(), Status: auditv3.ReceiptStatus_RECEIPT_STATUS_RECEIVED, Epoch: recEpoch,
			Seq: seq, Chain: recChain}, nil
	case err == nil:
		m.RT.Logger.Error("an audit record arrived again with another digest (integrity event)", "source", source, "id", it.GetId())
		return &auditv3.Receipt{Id: it.GetId(), Status: auditv3.ReceiptStatus_RECEIPT_STATUS_CONFLICT}, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}
	if *srcEpoch == it.GetEpoch() && it.GetSeq() != *srcLast+1 && it.GetSeq() > *srcLast {
		m.RT.Logger.Warn("a gap in a source's audit records: some were lost on the way", "source", source,
			"after", *srcLast, "got", it.GetSeq())
	}
	if *srcEpoch != it.GetEpoch() || it.GetSeq() > *srcLast {
		*srcEpoch, *srcLast = it.GetEpoch(), it.GetSeq()
	}
	*last++
	*chain = core.ChainNext(*chain, it.GetDigest())
	if _, err := tx.Exec(ctx, `INSERT INTO audit_record (source, id, digest, record, source_seq, source_epoch, epoch, storage_seq,
		chain, rkey) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`, source, it.GetId(), it.GetDigest(),
		it.GetRecord(), it.GetSeq(), it.GetEpoch(), epoch, *last, *chain, source+"/"+it.GetId()); err != nil {
		return nil, err
	}
	return &auditv3.Receipt{Id: it.GetId(), Status: auditv3.ReceiptStatus_RECEIPT_STATUS_RECEIVED, Epoch: epoch, Seq: *last,
		Chain: *chain}, nil
}

// Watermark is IntakeService.Watermark: nothing backed up answers seq 0, so a source deletes nothing.
func Watermark(m *core.Base, ctx context.Context, _ *auditv3.WatermarkRequest) (*auditv3.WatermarkResponse, error) {
	out := &auditv3.WatermarkResponse{}
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		epoch, err := core.EpochOf(ctx, tx)
		if err != nil {
			return err
		}
		out.Epoch = epoch
		var seq *int64
		if err := tx.QueryRow(ctx, "SELECT max(seq) FROM audit_backup WHERE epoch = $1", epoch).Scan(&seq); err != nil {
			return err
		}
		if seq != nil {
			out.Seq, out.BackedUpInEpoch = *seq, true
		}
		rows, err := tx.Query(ctx, "SELECT epoch, fork FROM audit_epoch WHERE epoch <> $1 ORDER BY closed_at", epoch)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a := &auditv3.Ancestor{}
			if err := rows.Scan(&a.Epoch, &a.Fork); err != nil {
				return err
			}
			out.Ancestors = append(out.Ancestors, a)
		}
		return rows.Err()
	})
	return out, err
}
