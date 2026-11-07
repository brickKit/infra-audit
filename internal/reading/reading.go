package reading

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	archive "github.com/brickKit/infra-audit/v3/internal/archive"
	core "github.com/brickKit/infra-audit/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/audit"
	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/httpx"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	auditv3 "github.com/brickKit/infra-audit/contract/v3/gen/go/be/audit/v3"
)

// RecordList answers RecordService (contract alpha.3) for the configured readers.
type RecordList struct {
	auditv3.UnimplementedRecordServiceServer
	M *core.Base
}

// List is RecordService.List: the records that occurred at or after since, by category, in the store's order after a
// position, 500 a page at most (a full recovery's loss range, assembly protocol 15 step 9). Only the online records:
// a range that reaches records which left the online table answers QUERY_NOT_COMPLETABLE (a recovery's range is
// days, the records stay online a year).
func (r RecordList) List(ctx context.Context, in *auditv3.RecordServiceListRequest) (*auditv3.RecordServiceListResponse, error) {
	since, err := time.Parse(time.RFC3339Nano, in.GetSince())
	if err != nil {
		return nil, errs.New(errs.ReasonRequestInvalid, "since is not an RFC 3339 time")
	}
	epoch, seq := "", int64(-1)
	if in.GetAfter() != "" {
		if epoch, seq, err = core.ParsePosition(in.GetAfter()); err != nil {
			return nil, errs.New(errs.ReasonRequestInvalid, err.Error())
		}
	}
	limit := int(in.GetLimit())
	if limit <= 0 || limit > core.MaxBatch {
		limit = core.MaxBatch
	}
	categories := in.GetCategories()
	out := &auditv3.RecordServiceListResponse{}
	err = r.M.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		out.Records = out.Records[:0]
		// records received in the range may have left the online table: never a part of them (A158, P15.26)
		var gone bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM be_tier_unit WHERE class = $1 AND state IN ('released',
			'destroyed') AND anchor_max >= $2::timestamptz - make_interval(secs => $3))`, archive.RecordClass, since,
			core.RecordSkew.Seconds()).Scan(&gone); err != nil {
			return err
		}
		if gone {
			return errs.New(errs.ReasonQueryNotCompletable, "records of the range have left the online table (archived)")
		}
		rows, err := tx.Query(ctx, `SELECT source, record, source_epoch, source_seq, epoch, storage_seq FROM audit_record
			WHERE received_at >= $1::timestamptz - make_interval(secs => $2)
			  AND (convert_from(record, 'UTF8')::jsonb ->> 'occurred_at')::timestamptz >= $1::timestamptz
			  AND (coalesce(cardinality($3::text[]), 0) = 0 OR convert_from(record, 'UTF8')::jsonb ->> 'category' = ANY($3))
			  AND (epoch, storage_seq) > ($4, $5)
			ORDER BY epoch, storage_seq LIMIT $6`, since, core.RecordSkew.Seconds(), categories, epoch, seq, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s auditv3.StoredRecord
			var ep string
			var sq int64
			if err := rows.Scan(&s.Source, &s.Record, &s.SourceEpoch, &s.SourceSeq, &ep, &sq); err != nil {
				return err
			}
			s.Position = core.Position(ep, sq)
			out.Records = append(out.Records, &s)
		}
		return rows.Err()
	})
	if len(out.Records) > limit {
		out.Records, out.More = out.Records[:limit], true
	}
	return out, err
}

// Query lists the latest records, optionally of one source and one operation (AU13): the caller's grant decides
// which fields come out (the others null and named under _masked), and the query itself is audited as a sensitive
// read before anything is released (P8.19: no record, no answer).
func Query(m *core.Base, c *httpx.Context) error {
	s, _ := sdkauthz.From(c.Ctx())
	ctx, d, err := s.Check(c.Ctx(), core.OpRead, sdkauthz.Facts{})
	if err != nil {
		return err
	}
	if c.Query("history") == "true" && m.RT.Tiering != nil {
		return archive.History(m, c, ctx, s)
	}
	limit := 100
	if n, err := strconv.Atoi(c.Query("limit")); err == nil && n > 0 && n <= 500 {
		limit = n
	}
	type row struct {
		Source     string          `json:"source"`
		ID         string          `json:"id"`
		Epoch      string          `json:"epoch"`
		Seq        int64           `json:"seq"`
		Record     json.RawMessage `json:"record"`
		ReceivedAt time.Time       `json:"received_at"`
	}
	var out []row
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Query(ctx, `SELECT source, id::text, epoch, storage_seq, record, received_at FROM audit_record
			WHERE ($1 = '' OR source = $1) AND ($2 = '' OR convert_from(record, 'UTF8')::jsonb->>'operation' = $2)
			ORDER BY received_at DESC, storage_seq DESC LIMIT $3`, c.Query("source"), c.Query("operation"), limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r row
			var raw []byte
			if err := rows.Scan(&r.Source, &r.ID, &r.Epoch, &r.Seq, &raw, &r.ReceivedAt); err != nil {
				return err
			}
			r.Record = raw
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	res := core.Definitions().Resources[0] // the audit records
	shaped := make([]map[string]any, 0, len(out))
	for _, r := range out {
		shaped = append(shaped, sdkauthz.Mask(&res, map[string]any{"source": r.Source, "id": r.ID, "record": r.Record,
			"received_at": r.ReceivedAt}, d.Fields))
	}
	if err := m.RT.Audit.Read(ctx, audit.Record{Operation: core.OpRead, Outcome: audit.Succeeded, Actor: s.Principal,
		Target: map[string]any{"type": core.Records}, Request: map[string]any{"source": c.Query("source"),
			"operation": c.Query("operation"), "returned": len(shaped)}}); err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"records": shaped})
}
