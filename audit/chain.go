package audit

import (
	"context"
	"fmt"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/records"
	"github.com/brickKit/be-sdk-go/v3/work"
	"github.com/jackc/pgx/v5"
)

// The chain check (protocol 03, AU14): every source's records in storage order, each one's bytes against its digest
// and the chain recomputed over the digests (contract/README.md: chainNext), the last one against the source's head.
// A record changed in place, taken out of the middle or off the end breaks it; the first break of each source is
// recorded (audit_chain_break) and logged as an integrity event. The check reads every record each time: a
// check that started after the last verified record would not see an earlier one changed.

// JobChainCheck verifies every source's chain at AUDIT_CHAIN_CHECK_INTERVAL.
const JobChainCheck = "infra.audit.chain_check"

// Jobs registers the chain check.
func (m *Module) Jobs(j *work.Jobs) error {
	j.Add(work.Job{Name: JobChainCheck, Kind: work.Singleton, Effect: work.Local, MaxAttempts: work.Unlimited,
		Interval: m.rt.Config.Duration("AUDIT_CHAIN_CHECK_INTERVAL"), Handle: m.chainCheck})
	return nil
}

type chainBreak struct {
	source, epoch, detail string
	seq                   int64
}

func (m *Module) chainCheck(ctx context.Context, _ *work.Run) error {
	var found []chainBreak
	err := m.rt.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		found = nil
		rows, err := tx.Query(ctx, "SELECT source, chain FROM audit_source ORDER BY source")
		if err != nil {
			return err
		}
		type head struct{ source, chain string }
		heads, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (head, error) {
			var h head
			return h, r.Scan(&h.source, &h.chain)
		})
		if err != nil {
			return err
		}
		for _, h := range heads {
			b, err := verifyChain(ctx, tx, h.source, h.chain)
			if err != nil {
				return err
			}
			if b != nil {
				found = append(found, *b)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, b := range found {
		m.rt.Logger.Error("an audit chain does not verify (integrity event)", "event", "audit_chain_broken",
			"source", b.source, "epoch", b.epoch, "seq", b.seq, "detail", b.detail)
	}
	if len(found) == 0 {
		return nil
	}
	return m.rt.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		for _, b := range found {
			if _, err := tx.Exec(ctx, `INSERT INTO audit_chain_break (source, at_epoch, at_seq, detail) VALUES ($1, $2, $3, $4)
				ON CONFLICT DO NOTHING`, b.source, b.epoch, b.seq, b.detail); err != nil {
				return err
			}
		}
		return nil
	})
}

// verifyChain walks one source's records in storage order (the audit database's epochs in order, then the storage
// sequence); nil when the chain holds up to its head.
func verifyChain(ctx context.Context, tx db.Tx, source, headChain string) (*chainBreak, error) {
	archived, err := releasedBefore(ctx, tx, source)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT epoch, storage_seq, digest, record, chain FROM audit_record WHERE source = $1
		ORDER BY split_part(epoch, '.', 1)::bigint, split_part(epoch, '.', 2)::bigint, storage_seq`, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	chain, epoch, seq, first := "", "", int64(0), true
	for rows.Next() {
		var digest, stored string
		var raw []byte
		if err := rows.Scan(&epoch, &seq, &digest, &raw, &stored); err != nil {
			return nil, err
		}
		if _, err := records.Open("be.audit.record", raw, digest); err != nil {
			return &chainBreak{source, epoch, "the record's bytes are not its digest: " + err.Error(), seq}, nil
		}
		next := chainNext(chain, digest)
		if first && next != stored && archived { // the records before it left the table (their unit's digest
			next = stored // protects them, A158): the chain starts here
		}
		first, chain = false, next
		if chain != stored {
			return &chainBreak{source, epoch, "the chain does not follow from the records before (one taken out or put in)", seq}, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if chain != headChain {
		return &chainBreak{source, epoch, fmt.Sprintf("the head is %s, the records give %s (taken off the end)", headChain, chain), seq}, nil
	}
	return nil, nil
}

// releasedBefore says whether a source has records whose online rows were released (archived or destroyed).
func releasedBefore(ctx context.Context, tx db.Tx, source string) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM be_tier_member m JOIN be_tier_unit u ON u.id = m.unit
		WHERE m.class = $1 AND u.state IN ('released', 'destroyed') AND left(m.root_id, length($2) + 1) = $2 || '/')`, RecordClass, source).Scan(&ok)
	return ok, err
}
