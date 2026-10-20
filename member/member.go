// Package member is the audit component (boundary card infra/audit): it takes every component's audit records over
// the system surface, stores them idempotently by source and ID with receipts, tells how far they are backed up,
// and answers queries (protocol 03; contract/README.md). Step 5 of the representative deployment experiment: the
// evidence store and the registration of backups come with protocol 15.
package member

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"strconv"
	"time"

	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/component"
	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/grpcx"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/records"
	"github.com/brickKit/be-sdk-go/v3/runtime"
	auditv3 "github.com/brickKit/infra-audit/contract/v3/gen/go/be/audit/v3"
	"github.com/jackc/pgx/v5"
)

// Identity of the component, its resources and operations.
const (
	ID          = "infra/audit"
	Version     = "3.0.2"
	GRPCPort    = 9206
	Records     = "infra.audit.record"
	OpRead      = "infra.audit.record.read"
	Intake      = "infra.audit.intake"
	OpSubmit    = "infra.audit.intake.submit"
	OpWatermark = "infra.audit.intake.watermark"
	IntakeRead  = "infra.audit.intake.read"
	MaxBatch    = 500
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func migrations() fs.FS {
	sub, _ := fs.Sub(migrationFiles, "migrations")
	return sub
}

// Definitions are the component's declarations.
func Definitions() *sdkauthz.Definitions {
	none := map[string]sdkauthz.Dimension{}
	return &sdkauthz.Definitions{Component: ID,
		Resources: []sdkauthz.Resource{
			{Type: Records, ViewOperation: OpRead, Dimensions: none, Fields: map[string]sdkauthz.Field{
				"source": {Class: sdkauthz.Ordinary}, "id": {Class: sdkauthz.Ordinary}, "record": {Class: sdkauthz.Ordinary},
				"received_at": {Class: sdkauthz.Ordinary}}},
			{Type: Intake, ViewOperation: IntakeRead, Dimensions: none, Fields: map[string]sdkauthz.Field{}}},
		Operations: []sdkauthz.Operation{
			{Key: OpRead, ResourceType: Records, Purpose: "special_read", RiskClass: "low"},
			{Key: IntakeRead, ResourceType: Intake, Purpose: "read", RiskClass: "low"},
			{Key: OpSubmit, ResourceType: Intake, Purpose: "action", RiskClass: "low", SideEffects: []string{"business_write"}},
			{Key: OpWatermark, ResourceType: Intake, Purpose: "read", RiskClass: "low"},
		}}
}

// Definition is the component as the launcher and the shells take it.
func Definition() component.Definition {
	return component.Definition{ID: ID, Version: Version, Migrations: migrations(), GRPCPort: GRPCPort, Authz: Definitions(),
		New: func(rt *runtime.Runtime) (component.Module, error) { return &Module{rt: rt}, nil }}
}

// Module is the audit component.
type Module struct {
	auditv3.UnimplementedIntakeServiceServer
	rt *runtime.Runtime
}

// Services registers the intake; every component of the deployment sends its records.
func (m *Module) Services(s *grpcx.Services) error {
	s.Register(&auditv3.IntakeService_ServiceDesc, m, map[string]grpcx.Method{
		"Submit":    {Operation: OpSubmit, Callers: []string{grpcx.AnyComponent}},
		"Watermark": {Operation: OpWatermark, Callers: []string{grpcx.AnyComponent}}})
	return nil
}

// Routes registers the query.
func (m *Module) Routes(r *httpx.Routes) error {
	r.Add(httpx.Route{Method: "GET", Path: "/api/audit/records", Operation: OpRead, Handle: m.query})
	return nil
}

func epochOf(ctx context.Context, tx db.Tx) (string, error) {
	var g, t int64
	if err := tx.QueryRow(ctx, "SELECT generation, timeline FROM be_current_db_epoch()").Scan(&g, &t); err != nil {
		return "", err
	}
	return strconv.FormatInt(g, 10) + "." + strconv.FormatInt(t, 10), nil
}

// chainNext is the source's chain digest after one more record (contract/README.md).
func chainNext(prev, digest string) string {
	sum := sha256.Sum256([]byte(prev + "\n" + digest))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Submit is IntakeService.Submit (contract/README.md, "审计组件的义务"): every item checked, stored once per source
// and ID under the storage counter's lock, a receipt for each in order; the answer leaves only after the commit.
func (m *Module) Submit(ctx context.Context, in *auditv3.SubmitRequest) (*auditv3.SubmitResponse, error) {
	source, _ := grpcx.Caller(ctx)
	if len(in.GetItems()) > MaxBatch {
		return nil, errs.New(errs.ReasonBatchTooLarge, "at most 500 records a call").GRPCStatus().Err()
	}
	out := &auditv3.SubmitResponse{}
	err := m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		out.Receipts = out.Receipts[:0]
		epoch, err := epochOf(ctx, tx)
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
			r, err := m.item(ctx, tx, source, epoch, &last, &chain, &srcEpoch, &srcLast, it)
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
func (m *Module) item(ctx context.Context, tx db.Tx, source, epoch string, last *int64, chain, srcEpoch *string,
	srcLast *int64, it *auditv3.Item) (*auditv3.Receipt, error) {
	rec, err := records.Open("be.audit.record", it.GetRecord(), it.GetDigest())
	if err != nil || rec["id"] != it.GetId() {
		m.rt.Logger.Error("an audit record was refused as invalid (integrity event)", "source", source, "id", it.GetId())
		return &auditv3.Receipt{Id: it.GetId(), Status: auditv3.ReceiptStatus_RECEIPT_STATUS_INVALID}, nil
	}
	var digest, recEpoch, recChain string
	var seq int64
	err = tx.QueryRow(ctx, "SELECT digest, epoch, storage_seq, chain FROM audit_record WHERE source = $1 AND id = $2",
		source, it.GetId()).Scan(&digest, &recEpoch, &seq, &recChain)
	switch {
	case err == nil && digest == it.GetDigest(): // stored before: the original receipt
		return &auditv3.Receipt{Id: it.GetId(), Status: auditv3.ReceiptStatus_RECEIPT_STATUS_RECEIVED, Epoch: recEpoch,
			Seq: seq, Chain: recChain}, nil
	case err == nil:
		m.rt.Logger.Error("an audit record arrived again with another digest (integrity event)", "source", source, "id", it.GetId())
		return &auditv3.Receipt{Id: it.GetId(), Status: auditv3.ReceiptStatus_RECEIPT_STATUS_CONFLICT}, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}
	if *srcEpoch == it.GetEpoch() && it.GetSeq() != *srcLast+1 && it.GetSeq() > *srcLast {
		m.rt.Logger.Warn("a gap in a source's audit records: some were lost on the way", "source", source,
			"after", *srcLast, "got", it.GetSeq())
	}
	if *srcEpoch != it.GetEpoch() || it.GetSeq() > *srcLast {
		*srcEpoch, *srcLast = it.GetEpoch(), it.GetSeq()
	}
	*last++
	*chain = chainNext(*chain, it.GetDigest())
	if _, err := tx.Exec(ctx, `INSERT INTO audit_record (source, id, digest, record, source_seq, source_epoch, epoch, storage_seq, chain)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, source, it.GetId(), it.GetDigest(), it.GetRecord(), it.GetSeq(),
		it.GetEpoch(), epoch, *last, *chain); err != nil {
		return nil, err
	}
	return &auditv3.Receipt{Id: it.GetId(), Status: auditv3.ReceiptStatus_RECEIPT_STATUS_RECEIVED, Epoch: epoch, Seq: *last,
		Chain: *chain}, nil
}

// Watermark is IntakeService.Watermark: nothing backed up answers seq 0, so a source deletes nothing.
func (m *Module) Watermark(ctx context.Context, _ *auditv3.WatermarkRequest) (*auditv3.WatermarkResponse, error) {
	out := &auditv3.WatermarkResponse{}
	err := m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		epoch, err := epochOf(ctx, tx)
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

// query lists the latest records, optionally of one source and one operation.
func (m *Module) query(c *httpx.Context) error {
	s, _ := sdkauthz.From(c.Ctx())
	ctx, _, err := s.Check(c.Ctx(), OpRead, sdkauthz.Facts{})
	if err != nil {
		return err
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
	err = m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
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
	return c.JSON(200, map[string]any{"records": out})
}
