//go:build integration

package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/brickKit/be-sdk-go/v3/canon"
	"github.com/brickKit/be-sdk-go/v3/config"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/be-sdk-go/v3/sectest"
	auditv3 "github.com/brickKit/infra-audit/contract/v3/gen/go/be/audit/v3"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

const evSchema = "audit_it_ev"

// evidenceRecord is one be.evidence.record as a recorder seals it (P12.9).
func evidenceRecord(t *testing.T, recorder, id, phase, subject string, change map[string]any) *auditv3.Evidence {
	r := map[string]any{"record_type": "be.evidence.record", "schema": "1.0", "id": id,
		"recorder": map[string]any{"component": recorder, "instance": recorder + "-1"}, "category": "security_tightening",
		"phase": phase, "subject": subject,
		"object": map[string]any{"type": "demo.order", "id": "0190f3a1-0000-7000-8000-0000000000f1", "version": "3"},
		"change": change, "at": canon.Time(time.Now())}
	if phase == "effective" || phase == "general" { // only these commit with the business transaction (P12.9, alpha.74)
		r["position"] = map[string]any{"epoch": "1.1", "commit_seq": 107}
	}
	raw, err := canon.Encode(r)
	require.NoError(t, err)
	return &auditv3.Evidence{Record: raw, Digest: canon.DigestBytes("be.evidence.record", 1, raw)}
}

// withPosition re-seals a record with a position it must not carry.
func withPosition(t *testing.T, e *auditv3.Evidence) *auditv3.Evidence {
	var r map[string]any
	require.NoError(t, json.Unmarshal(e.GetRecord(), &r))
	r["position"] = map[string]any{"epoch": "1.1", "commit_seq": 9}
	raw, err := canon.Encode(r)
	require.NoError(t, err)
	return &auditv3.Evidence{Record: raw, Digest: canon.DigestBytes("be.evidence.record", 1, raw)}
}

// TestTheEvidenceStore: kept once by (id, phase), conclusions matched to their intents, the recorder is the caller,
// receipts after the commit, listed in the store's order for the configured readers only (P12.9-P12.13).
func TestTheEvidenceStore(t *testing.T) {
	pgPassword := os.Getenv("AUDIT_IT_PG_PASSWORD")
	if pgPassword == "" {
		t.Fatal("AUDIT_IT_PG_PASSWORD is not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, "postgres://postgres:"+url.QueryEscape(pgPassword)+"@localhost:5432/"+itDB+"?sslmode=disable")
	require.NoError(t, err)
	defer admin.Close(ctx)
	_, err = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+evSchema+" CASCADE; CREATE SCHEMA "+evSchema)
	require.NoError(t, err)
	w := sectest.New(t)
	portFree(t, itPort, itSystem)
	env := w.Env(t, ID)
	for k, v := range map[string]string{"COMPONENT_ID": ID, "COMPONENT_VERSION": Version, "PORT": itPort, "STOP_DRAIN_DELAY": "50ms",
		"PG_HOST": "localhost", "PG_DATABASE": itDB, "PG_USER": "postgres", "PG_PASSWORD": pgPassword, "PG_SCHEMA": evSchema,
		"PG_OWNER_USER": "postgres", "PG_OWNER_PASSWORD": pgPassword} {
		env[k] = v
	}
	var logs bytes.Buffer
	opts := launcher.Options{Stdout: &logs, Stderr: &logs,
		Env: config.Env{LookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok }, Read: os.ReadFile}}
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(logs.String())
		}
	})
	migrate := opts
	migrate.Args = []string{"migrate"}
	require.Equal(t, launcher.ExitOK, launcher.Run(ctx, Definition(), migrate))
	sctx, stop := context.WithCancel(ctx)
	done := make(chan int, 1)
	go func() { done <- launcher.Run(sctx, Definition(), opts) }()
	defer func() { stop(); <-done }()
	require.Eventually(t, func() bool {
		r, err := http.Get(itBase + "/readyz")
		if err == nil {
			r.Body.Close()
		}
		return err == nil && r.StatusCode == 200
	}, 15*time.Second, 100*time.Millisecond)

	client := func(component string) auditv3.EvidenceServiceClient {
		tls := w.TLS(t, component)
		conn, err := grpc.NewClient(itSystem, grpc.WithTransportCredentials(credentials.NewTLS(tls.ClientConfig(ID))))
		require.NoError(t, err)
		t.Cleanup(func() { conn.Close() })
		return auditv3.NewEvidenceServiceClient(conn)
	}
	orders, recover := client("demo/orders"), client(DefaultEvidenceReader)
	appendAs := func(c auditv3.EvidenceServiceClient, recs ...*auditv3.Evidence) ([]*auditv3.EvidenceReceipt, error) {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		out, err := c.Append(cctx, &auditv3.AppendRequest{Records: recs})
		return out.GetReceipts(), err
	}
	const (
		accepted  = auditv3.EvidenceStatus_EVIDENCE_STATUS_ACCEPTED
		duplicate = auditv3.EvidenceStatus_EVIDENCE_STATUS_DUPLICATE
		conflict  = auditv3.EvidenceStatus_EVIDENCE_STATUS_CONFLICT
		invalid   = auditv3.EvidenceStatus_EVIDENCE_STATUS_INVALID
	)
	bob, carol := sectest.Bob, "0190f3a1-0000-7000-8000-0000000000c3"
	id1, id2, id3 := ids.New().String(), ids.New().String(), ids.New().String()
	intent1 := evidenceRecord(t, "demo/orders", id1, "intent", bob, map[string]any{"termination": "revoke"})
	bad := evidenceRecord(t, "demo/orders", ids.New().String(), "intent", bob, map[string]any{"termination": "revoke"})
	bad.Digest = "sha256:" + string(bytes.Repeat([]byte("0"), 64))
	rs, err := appendAs(orders,
		intent1, // kept
		intent1, // the same again in one batch: the original receipt
		evidenceRecord(t, "demo/orders", id1, "intent", bob, map[string]any{"termination": "narrow"}),    // other bytes
		evidenceRecord(t, "demo/orders", id1, "effective", bob, map[string]any{"termination": "revoke"}), // matches
		evidenceRecord(t, "demo/orders", id2, "intent", bob, map[string]any{"termination": "revoke"}),
		evidenceRecord(t, "demo/orders", id2, "effective", carol, map[string]any{"termination": "revoke"}), // another subject
		evidenceRecord(t, "demo/orders", id3, "cancelled", bob, map[string]any{"termination": "revoke"}),   // no intent kept
		bad,
		withPosition(t, evidenceRecord(t, "demo/orders", ids.New().String(), "cancelled", bob, map[string]any{"x": "y"}))) // never committed
	require.NoError(t, err)
	require.Len(t, rs, 9)
	want := []auditv3.EvidenceStatus{accepted, duplicate, conflict, accepted, accepted, conflict, accepted, invalid, invalid}
	for i, r := range rs {
		assert.Equal(t, want[i], r.GetStatus(), "record %d", i)
	}
	assert.EqualValues(t, 1, rs[0].GetSeq())
	assert.Equal(t, rs[0].GetSeq(), rs[1].GetSeq(), "a duplicate answers the original position")
	assert.EqualValues(t, 2, rs[3].GetSeq())
	assert.NotEmpty(t, rs[0].GetEpoch())
	var kept int
	require.NoError(t, admin.QueryRow(ctx, "SELECT count(*) FROM "+evSchema+".audit_evidence").Scan(&kept))
	assert.Equal(t, 4, kept, "kept by the time the answer came (P12.12)")
	assert.Contains(t, logs.String(), `"event":"evidence_conflict"`, "a conflict is an integrity event")

	// the recorder is the caller: another component's record is refused, the whole batch
	_, err = appendAs(orders, evidenceRecord(t, "demo/orders", ids.New().String(), "intent", bob, map[string]any{"x": "y"}),
		evidenceRecord(t, "infra/authz", ids.New().String(), "intent", bob, map[string]any{"x": "y"}))
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "%v", err)
	require.NoError(t, admin.QueryRow(ctx, "SELECT count(*) FROM "+evSchema+".audit_evidence").Scan(&kept))
	assert.Equal(t, 4, kept, "nothing of the refused batch")

	// listing: the configured readers only, in the store's order, paged, filtered
	list := func(c auditv3.EvidenceServiceClient, req *auditv3.ListRequest) (*auditv3.ListResponse, error) {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return c.List(cctx, req)
	}
	_, err = list(orders, &auditv3.ListRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "a recorder is no reader: %v", err)
	page, err := list(recover, &auditv3.ListRequest{Limit: 3})
	require.NoError(t, err)
	require.Len(t, page.GetRecords(), 3)
	assert.True(t, page.GetMore())
	assert.EqualValues(t, []int64{1, 2, 3}, []int64{page.Records[0].GetSeq(), page.Records[1].GetSeq(), page.Records[2].GetSeq()})
	assert.Equal(t, intent1.GetRecord(), page.Records[0].GetEvidence().GetRecord(), "the bytes as received")
	page, err = list(recover, &auditv3.ListRequest{AfterSeq: 3})
	require.NoError(t, err)
	require.Len(t, page.GetRecords(), 1)
	assert.False(t, page.GetMore())
	page, err = list(recover, &auditv3.ListRequest{Recorder: "infra/authz"})
	require.NoError(t, err)
	assert.Empty(t, page.GetRecords())
	page, err = list(recover, &auditv3.ListRequest{Category: "security_tightening"})
	require.NoError(t, err)
	assert.Len(t, page.GetRecords(), 4)
}
