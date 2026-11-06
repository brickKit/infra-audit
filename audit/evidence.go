package audit

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/brickKit/be-sdk-go/v3/canon"
	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/grpcx"
	auditv3 "github.com/brickKit/infra-audit/contract/v3/gen/go/be/audit/v3"
	"github.com/jackc/pgx/v5"
)

// The evidence store (component protocol P12.9-P12.14, contract/README.md "EvidenceService"): a full restore must not
// lose these records; they are kept apart from the audit records, once by (id, phase), and listed for the readers
// the deployment names.

// EvidenceRecordType is every evidence record's type (P12.9).
const EvidenceRecordType = "be.evidence.record"

// Categories are the rows of assembly protocol 15's evidence table (the evidence-record vectors).
var Categories = []string{"security_tightening", "security_widening", "employment_end", "org_tightening",
	"account_status", "account_reenable", "identity_signing_key_revocation", "credential_tightening",
	"credential_tightening_in_product", "session_revocation", "recovery_credential_rotation", "document_number",
	"member_switch_handover", "destruction", "erasure", "external_command_issued", "external_command_outcome",
	"deployment_generation", "authz_generation", "evidence_double_failure"}

// evidence is the part of a record the store checks and compares (P12.9, P12.10).
type evidence struct {
	RecordType string `json:"record_type"`
	Schema     string `json:"schema"`
	ID         string `json:"id"`
	Recorder   struct {
		Component string `json:"component"`
		Instance  string `json:"instance"`
	} `json:"recorder"`
	Category string `json:"category"`
	Phase    string `json:"phase"`
	Position *struct {
		Epoch     string `json:"epoch"`
		CommitSeq int64  `json:"commit_seq"`
	} `json:"position"`
	Subject string          `json:"subject"`
	Object  json.RawMessage `json:"object"`
	Change  json.RawMessage `json:"change"`
	At      string          `json:"at"`
}

// parseEvidence checks one record: canonical bytes, its digest, the fields (P12.9). An error is INVALID.
func parseEvidence(raw []byte, digest string) (evidence, error) {
	var e evidence
	if _, err := canon.Parse(raw); err != nil {
		return e, err
	}
	if canon.DigestBytes(EvidenceRecordType, 1, raw) != digest {
		return e, errors.New("the digest is not the record's")
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return e, err
	}
	switch {
	case e.RecordType != EvidenceRecordType || e.Schema != "1.0":
		return e, errors.New("not a be.evidence.record 1.0")
	case !uuidText(e.ID) || e.Recorder.Component == "" || e.Recorder.Instance == "":
		return e, errors.New("the id and the recorder are required")
	case !slices.Contains(Categories, e.Category):
		return e, errors.New("an unknown category")
	case !slices.Contains([]string{"intent", "effective", "cancelled", "general"}, e.Phase):
		return e, errors.New("an unknown phase")
	case (e.Phase == "effective" || e.Phase == "general") != (e.Position != nil):
		return e, errors.New("effective and general records carry a position; an intent or a cancelled one does not")
	case len(e.Object) == 0 || len(e.Change) == 0:
		return e, errors.New("the object and the change are required")
	}
	if _, err := time.Parse(time.RFC3339Nano, e.At); err != nil {
		return e, errors.New("at is not an instant")
	}
	return e, nil
}

func uuidText(s string) bool {
	return len(s) == 36 && strings.Count(s, "-") == 4
}

// sameSubject: a conclusion matches its intent (P12.10).
func sameSubject(a, b evidence) bool {
	return a.Recorder == b.Recorder && a.Category == b.Category && a.Subject == b.Subject && string(a.Object) == string(b.Object)
}

// Append is EvidenceService.Append: the recorder is the caller (P12.11), each record kept once by (id, phase)
// (P12.10), all in one transaction under the counter's lock, answered after the commit (P12.12).
func (m *Module) Append(ctx context.Context, in *auditv3.AppendRequest) (*auditv3.AppendResponse, error) {
	caller, _ := grpcx.Caller(ctx)
	if len(in.GetRecords()) > MaxBatch {
		return nil, errs.New(errs.ReasonBatchTooLarge, "at most 500 records a call")
	}
	parsed := make([]evidence, len(in.GetRecords()))
	bad := make([]error, len(in.GetRecords()))
	for i, r := range in.GetRecords() {
		parsed[i], bad[i] = parseEvidence(r.GetRecord(), r.GetDigest())
		if bad[i] == nil && parsed[i].Recorder.Component != caller {
			return nil, errs.New(errs.ReasonPermissionDenied, "a component appends its own evidence only").
				With("recorder", parsed[i].Recorder.Component)
		}
	}
	out := &auditv3.AppendResponse{}
	err := m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		out.Receipts = out.Receipts[:0]
		epoch, err := epochOf(ctx, tx)
		if err != nil {
			return err
		}
		var last int64
		if err := tx.QueryRow(ctx, "SELECT last_seq FROM audit_evidence_counter FOR UPDATE").Scan(&last); err != nil {
			return err
		}
		for i, r := range in.GetRecords() {
			rc, err := m.keep(ctx, tx, epoch, &last, parsed[i], bad[i], r)
			if err != nil {
				return err
			}
			out.Receipts = append(out.Receipts, rc)
		}
		_, err = tx.Exec(ctx, "UPDATE audit_evidence_counter SET last_seq = $1", last)
		return err
	})
	return out, err
}

// keep stores one record or tells why not.
func (m *Module) keep(ctx context.Context, tx db.Tx, epoch string, last *int64, e evidence, bad error,
	r *auditv3.Evidence) (*auditv3.EvidenceReceipt, error) {
	rc := &auditv3.EvidenceReceipt{Id: e.ID, Phase: e.Phase}
	if bad != nil {
		rc.Status = auditv3.EvidenceStatus_EVIDENCE_STATUS_INVALID
		m.rt.Logger.Warn("an evidence record is not valid", "error", bad.Error())
		return rc, nil
	}
	var digest, gotEpoch string
	var seq int64
	err := tx.QueryRow(ctx, "SELECT digest, epoch, seq FROM audit_evidence WHERE id = $1 AND phase = $2", e.ID, e.Phase).
		Scan(&digest, &gotEpoch, &seq)
	switch {
	case err == nil && digest == r.GetDigest():
		rc.Status, rc.Epoch, rc.Seq = auditv3.EvidenceStatus_EVIDENCE_STATUS_DUPLICATE, gotEpoch, seq
		return rc, nil
	case err == nil:
		return m.conflict(rc, e, "another digest under the same id and phase"), nil
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}
	if e.Phase == "effective" || e.Phase == "cancelled" {
		var raw []byte
		err := tx.QueryRow(ctx, "SELECT record FROM audit_evidence WHERE id = $1 AND phase = 'intent'", e.ID).Scan(&raw)
		if err == nil {
			var intent evidence
			if json.Unmarshal(raw, &intent) != nil || !sameSubject(intent, e) {
				return m.conflict(rc, e, "the conclusion does not match its intent"), nil
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	*last++
	if _, err := tx.Exec(ctx, `INSERT INTO audit_evidence (id, phase, recorder, category, digest, record, epoch, seq)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, e.ID, e.Phase, e.Recorder.Component, e.Category, r.GetDigest(),
		r.GetRecord(), epoch, *last); err != nil {
		return nil, err
	}
	rc.Status, rc.Epoch, rc.Seq = auditv3.EvidenceStatus_EVIDENCE_STATUS_ACCEPTED, epoch, *last
	return rc, nil
}

func (m *Module) conflict(rc *auditv3.EvidenceReceipt, e evidence, why string) *auditv3.EvidenceReceipt {
	m.rt.Logger.Error("an evidence record conflicts with what the store keeps: an integrity event", "event",
		"evidence_conflict", "id", e.ID, "phase", e.Phase, "recorder", e.Recorder.Component, "why", why)
	rc.Status = auditv3.EvidenceStatus_EVIDENCE_STATUS_CONFLICT
	return rc
}

// List is EvidenceService.List: the store's order after a position, 500 a page at most, by recorder and category
// (P12.13); only the configured readers reach it (Services).
func (m *Module) List(ctx context.Context, in *auditv3.ListRequest) (*auditv3.ListResponse, error) {
	limit := int(in.GetLimit())
	if limit <= 0 || limit > MaxBatch {
		limit = MaxBatch
	}
	out := &auditv3.ListResponse{}
	err := m.rt.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		out.Records = out.Records[:0]
		rows, err := tx.Query(ctx, `SELECT record, digest, epoch, seq, received_at FROM audit_evidence
			WHERE seq > $1 AND ($2 = '' OR recorder = $2) AND ($3 = '' OR category = $3) ORDER BY seq LIMIT $4`,
			in.GetAfterSeq(), in.GetRecorder(), in.GetCategory(), limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s auditv3.StoredEvidence
			var ev auditv3.Evidence
			var rec []byte
			var digest string
			var at time.Time
			if err := rows.Scan(&rec, &digest, &s.Epoch, &s.Seq, &at); err != nil {
				return err
			}
			ev.Record, ev.Digest = rec, digest
			s.Evidence, s.ReceivedAt = &ev, canon.Time(at)
			out.Records = append(out.Records, &s)
		}
		return rows.Err()
	})
	if len(out.Records) > limit {
		out.Records, out.More = out.Records[:limit], true
	}
	return out, err
}
