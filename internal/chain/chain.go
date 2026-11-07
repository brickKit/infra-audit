package chain

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	archive "github.com/brickKit/infra-audit/v3/internal/archive"
	core "github.com/brickKit/infra-audit/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/records"
	"github.com/brickKit/be-sdk-go/v3/tiering"
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

type chainBreak struct {
	source, epoch, detail string
	seq                   int64
}

type head struct{ source, chain string }

func ChainCheck(m *core.Base, ctx context.Context, _ *work.Run) error {
	var found []chainBreak
	var archivedOnly []head // every record left the online table: the archive answers for the end of the chain
	err := m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		found, archivedOnly = nil, nil
		rows, err := tx.Query(ctx, "SELECT source, chain FROM audit_source ORDER BY source")
		if err != nil {
			return err
		}
		heads, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (head, error) {
			var h head
			return h, r.Scan(&h.source, &h.chain)
		})
		if err != nil {
			return err
		}
		for _, h := range heads {
			b, none, err := verifyChain(ctx, tx, h.source, h.chain)
			if err != nil {
				return err
			}
			if b != nil {
				found = append(found, *b)
			}
			if none {
				archivedOnly = append(archivedOnly, h)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, h := range archivedOnly { // outside the transaction: a store read never runs inside one
		chain, ok, err := lastArchivedChain(m, ctx, h.source)
		if err != nil {
			return err
		}
		if ok && chain != h.chain {
			found = append(found, chainBreak{h.source, "", fmt.Sprintf("the head is %s, the archive's last record gives %s "+
				"(taken off the end)", h.chain, chain), 0})
		}
	}
	for _, b := range found {
		m.RT.Logger.Error("an audit chain does not verify (integrity event)", "event", "audit_chain_broken",
			"source", b.source, "epoch", b.epoch, "seq", b.seq, "detail", b.detail)
	}
	if len(found) == 0 {
		return nil
	}
	return m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
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
// sequence); nil when the chain holds up to its head. archivedOnly: no record is online and some were released, so the
// head is the archive's to check (lastArchivedChain).
func verifyChain(ctx context.Context, tx db.Tx, source, headChain string) (_ *chainBreak, archivedOnly bool, _ error) {
	archived, err := releasedBefore(ctx, tx, source)
	if err != nil {
		return nil, false, err
	}
	rows, err := tx.Query(ctx, `SELECT epoch, storage_seq, digest, record, chain FROM audit_record WHERE source = $1
		ORDER BY split_part(epoch, '.', 1)::bigint, split_part(epoch, '.', 2)::bigint, storage_seq`, source)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	chain, epoch, seq, first := "", "", int64(0), true
	for rows.Next() {
		var digest, stored string
		var raw []byte
		if err := rows.Scan(&epoch, &seq, &digest, &raw, &stored); err != nil {
			return nil, false, err
		}
		if _, err := records.Open("be.audit.record", raw, digest); err != nil {
			return &chainBreak{source, epoch, "the record's bytes are not its digest: " + err.Error(), seq}, false, nil
		}
		next := core.ChainNext(chain, digest)
		if first && next != stored && archived { // the records before it left the table (their unit's digest
			next = stored // protects them, A158): the chain starts here
		}
		first, chain = false, next
		if chain != stored {
			return &chainBreak{source, epoch, "the chain does not follow from the records before (one taken out or put in)", seq}, false, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if first && archived {
		return nil, true, nil
	}
	if chain != headChain {
		return &chainBreak{source, epoch, fmt.Sprintf("the head is %s, the records give %s (taken off the end)", headChain, chain), seq}, false, nil
	}
	return nil, false, nil
}

// lastArchivedChain is the chain of a source's last record when every record left the online table: the record of the
// last storage position in the source's newest unit, read from the archive. ok=false: that unit was destroyed at the
// end of its retention (nothing answers for it any more), or there is no catalog.
func lastArchivedChain(m *core.Base, ctx context.Context, source string) (chain string, ok bool, err error) {
	if m.RT.Tiering == nil {
		return "", false, nil
	}
	var unit, state string
	err = m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.QueryRow(ctx, `SELECT u.id::text, u.state FROM be_tier_member mb JOIN be_tier_unit u ON u.id = mb.unit
			WHERE mb.class = $1 AND u.state IN ('released', 'destroyed') AND left(mb.root_id, length($2) + 1) = $2 || '/'
			ORDER BY u.anchor_max DESC, u.id DESC LIMIT 1`, archive.RecordClass, source).Scan(&unit, &state)
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && state != tiering.StateReleased) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	s, err := m.RT.Tiering.Snapshot(ctx, "the audit chain check")
	if err != nil {
		return "", false, err
	}
	defer func() { err = errors.Join(err, m.RT.Tiering.End(context.WithoutCancel(ctx), s)) }()
	aggs, err := m.RT.Tiering.ReadUnit(ctx, s, unit, nil)
	if err != nil {
		return "", false, err
	}
	var last [3]int64 // the epoch's two parts and the storage sequence: the order verifyChain walks
	for _, a := range aggs {
		var at [3]int64
		major, minor, _ := strings.Cut(fmt.Sprint(a.Root["epoch"]), ".")
		at[0], _ = strconv.ParseInt(major, 10, 64)
		at[1], _ = strconv.ParseInt(minor, 10, 64)
		at[2], _ = strconv.ParseInt(fmt.Sprint(a.Root["storage_seq"]), 10, 64)
		if fmt.Sprint(a.Root["source"]) == source && (!ok || slices.Compare(at[:], last[:]) > 0) {
			last, chain, ok = at, fmt.Sprint(a.Root["chain"]), true
		}
	}
	return chain, ok, nil
}

// releasedBefore says whether a source has records whose online rows were released (archived or destroyed).
func releasedBefore(ctx context.Context, tx db.Tx, source string) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM be_tier_member m JOIN be_tier_unit u ON u.id = m.unit
		WHERE m.class = $1 AND u.state IN ('released', 'destroyed') AND left(m.root_id, length($2) + 1) = $2 || '/')`, archive.RecordClass, source).Scan(&ok)
	return ok, err
}
