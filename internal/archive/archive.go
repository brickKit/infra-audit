package archive

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	core "github.com/brickKit/infra-audit/v3/internal/core"

	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"

	"github.com/brickKit/be-sdk-go/v3/audit"
	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/tiering"
	auditv3 "github.com/brickKit/infra-audit/contract/v3/gen/go/be/audit/v3"
	"github.com/jackc/pgx/v5"
)

// The audit records as a data class (assembly protocol 11, protocol 03's retention and legal hold, decision A158):
// grouped by source and month of reception, every record final, kept online a year, then on the class's route.
// The retention depends on the category, the legal entity's region and the customer: pending here, given at
// deployment (TIERING_RETENTION_INFRA_AUDIT_RECORDS). No erasable column: an erasure request pseudonymizes the
// subject's references, by rules the data lifecycle sets (protocol 03's open item), never the audit fact.
const (
	RecordClass = "infra.audit.records"
	// Residence is how long a record stays in the online table (days).
	Residence = 365
)

// RecordsClass is the class.
func RecordsClass() tiering.Class {
	return tiering.Class{Name: RecordClass, Category: tiering.CategoryAudit,
		Root: tiering.Table{Name: "audit_record", PK: "rkey", Columns: map[string]string{
			"rkey": tiering.TypeText, "source": tiering.TypeText, "id": tiering.TypeUUID, "digest": tiering.TypeText,
			"record": tiering.TypeBytea, "source_seq": tiering.TypeInt8, "source_epoch": tiering.TypeText,
			"epoch": tiering.TypeText, "storage_seq": tiering.TypeInt8, "chain": tiering.TypeText,
			"received_at": tiering.TypeTimestamptz}},
		Group: []string{"source"}, Anchor: "received_at", Created: "received_at", Terminal: "true",
		ResidenceDays: Residence, Retention: tiering.Retention{Pending: true}, Locate: []string{"source"},
		Routes: []string{tiering.RoutePostgres}, SchemaVersion: "1", HoldOperation: core.OpHold,
		ApproveOperation: core.OpApproveDelete}
}

// StoredRecord is what a receipt needs of a stored record.
type StoredRecord struct {
	Digest, Epoch, Chain string `json:"-"`
	Seq                  int64  `json:"-"`
}

// Archived answers, for a batch's items, the stored records whose online rows were released: read from the archive
// before the intake's transaction (a store read never runs inside it). A record released between this read and the
// transaction makes the insert fail on the write barrier; the source sends again and this read finds it.
func Archived(m *core.Base, ctx context.Context, source string, items []*auditv3.Item) (map[string]*StoredRecord, error) {
	if m.RT.Tiering == nil || len(items) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(items))
	for _, it := range items {
		keys = append(keys, source+"/"+it.GetId())
	}
	var released []string
	if err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Query(ctx, `SELECT m.root_id FROM be_tier_member m JOIN be_tier_unit u ON u.id = m.unit
			WHERE m.class = $1 AND m.root_id = ANY($2) AND u.state IN ('released', 'destroyed')`, RecordClass, keys)
		if err != nil {
			return err
		}
		released, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	}); err != nil {
		return nil, err
	}
	out := map[string]*StoredRecord{}
	for _, key := range released {
		a, found, err := m.RT.Tiering.Get(ctx, RecordClass, key)
		if errs.Is(err, "be", errs.ReasonNotFound) || (err == nil && !found) {
			continue // destroyed at the end of its retention (be-protocol P15.29): nothing answers for it any more
		}
		if err != nil {
			return nil, err
		}
		seq, err := strconv.ParseInt(fmt.Sprint(a.Root["storage_seq"]), 10, 64)
		if err != nil {
			return nil, err
		}
		out[fmt.Sprint(a.Root["id"])] = &StoredRecord{Digest: fmt.Sprint(a.Root["digest"]), Epoch: fmt.Sprint(a.Root["epoch"]),
			Chain: fmt.Sprint(a.Root["chain"]), Seq: seq}
	}
	return out, nil
}

// historyParams are the query's filters, for the online part and the archived records alike.
type historyParams struct {
	Source    string `json:"source"`
	Operation string `json:"operation"`
}

// shaped is a record as the query answers it, masked to the caller's fields (AU13).
func shaped(source, id string, record []byte, receivedAt string, fields []string) map[string]any {
	res := core.Definitions().Resources[0] // the audit records
	return sdkauthz.Mask(&res, map[string]any{"source": source, "id": id, "record": json.RawMessage(record),
		"received_at": receivedAt}, fields)
}

// Reader reads the audit records' history (P15.26): the online part with the query's filters, the archived records
// selected by the same filters; no dimension, so every record is visible to whoever may query.
func Reader(m *core.Base) tiering.Reader {
	cl := RecordsClass()
	return tiering.Reader{Op: core.OpRead,
		Facts: func(map[string]any) sdkauthz.Facts { return sdkauthz.Facts{} },
		Match: func(root map[string]any, raw json.RawMessage) bool {
			var p historyParams
			_ = json.Unmarshal(raw, &p)
			if p.Source != "" && root["source"] != p.Source {
				return false
			}
			if p.Operation == "" {
				return true
			}
			rec, err := base64.StdEncoding.DecodeString(fmt.Sprint(root["record"]))
			var op struct{ Operation string }
			return err == nil && json.Unmarshal(rec, &op) == nil && op.Operation == p.Operation
		},
		Output: func(a tiering.Aggregate, d sdkauthz.Decision) (map[string]any, error) {
			rec, err := base64.StdEncoding.DecodeString(fmt.Sprint(a.Root["record"]))
			if err != nil {
				return nil, err
			}
			return shaped(fmt.Sprint(a.Root["source"]), fmt.Sprint(a.Root["id"]), rec, fmt.Sprint(a.Root["received_at"]),
				d.Fields), nil
		},
		Online: func(ctx context.Context, tx db.Tx, s tiering.Snapshot, sub *sdkauthz.Subject, raw json.RawMessage,
			emit func(map[string]any) error) error {
			var p historyParams
			_ = json.Unmarshal(raw, &p)
			ctx, decide, err := sub.Decider(ctx, core.OpRead)
			if err != nil {
				return err
			}
			fields := decide(sdkauthz.Facts{}).Fields
			rows, err := tx.Query(ctx, `SELECT source, id::text, record, received_at FROM audit_record r
				WHERE ($1 = '' OR source = $1) AND ($2 = '' OR convert_from(record, 'UTF8')::jsonb->>'operation' = $2)
				AND `+s.OnlineExclusion(cl, "r"), p.Source, p.Operation)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var source, id string
				var rec []byte
				var at time.Time
				if err := rows.Scan(&source, &id, &rec, &at); err != nil {
					return err
				}
				if err := emit(shaped(source, id, rec, at.UTC().Format("2006-01-02T15:04:05.000000Z"), fields)); err != nil {
					return err
				}
			}
			return rows.Err()
		}}
}

// History answers the query over every record of the filters, the archived ones included (P15.26): complete within
// the limit, else 202 with the export it became (its status at /be/tiering/infra.audit.records/exports/{id}); the
// query is audited as a sensitive read before anything is released, as the online one.
func History(m *core.Base, c *httpx.Context, ctx context.Context, s *sdkauthz.Subject) error {
	params, _ := json.Marshal(historyParams{Source: c.Query("source"), Operation: c.Query("operation")})
	out, pending, err := m.RT.Tiering.History(ctx, s, RecordClass, params)
	if err != nil {
		return err
	}
	returned := len(out)
	if pending != nil {
		returned = -1
	}
	if err := m.RT.Audit.Read(ctx, audit.Record{Operation: core.OpRead, Outcome: audit.Succeeded, Actor: s.Principal,
		Target: map[string]any{"type": core.Records}, Request: map[string]any{"source": c.Query("source"),
			"operation": c.Query("operation"), "history": true, "returned": returned}}); err != nil {
		return err
	}
	if pending != nil {
		return c.JSON(http.StatusAccepted, map[string]string{"export": pending.Export})
	}
	sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i]["received_at"]) > fmt.Sprint(out[j]["received_at"]) })
	return c.JSON(http.StatusOK, map[string]any{"records": out})
}
