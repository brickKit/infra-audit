//go:build integration

package audit

// The audit component against PostgreSQL over its real surfaces: a source component's records sent over mTLS with
// the SDK's own sender, the receipts, the watermark, a stop and restart, and the protected query (be-sdk-go's
// sectest gives the certificates, tokens and the authorization member the query is guarded by). Needs
// AUDIT_IT_PG_PASSWORD (the postgres superuser of be-postgres); missing, it fails. The member listens on 28206 (user)
// and 9206 (system). Run: go test -tags integration ./member/

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	sdkaudit "github.com/brickKit/be-sdk-go/v3/audit"
	"github.com/brickKit/be-sdk-go/v3/canon"
	"github.com/brickKit/be-sdk-go/v3/config"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/be-sdk-go/v3/records"
	"github.com/brickKit/be-sdk-go/v3/sectest"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	itPort   = "28206"
	itBase   = "http://localhost:" + itPort
	itSystem = "localhost:9206"
	itSchema = "audit_it"
	itDB     = "brickkit_test_db"
)

// wire is one record as the SDK sends it: canonical bytes stored with the row, its digest, the source's sequence.
func wire(t *testing.T, seq int64, op string) sdkaudit.Wire {
	id := ids.New().String()
	raw, digest, err := records.Seal("be.audit.record", map[string]any{"id": id, "occurred_at": canon.Time(time.Now()),
		"category": "business_change", "operation": op, "outcome": "succeeded", "actor": sectest.Alice})
	require.NoError(t, err)
	return sdkaudit.Wire{ID: id, Seq: seq, Epoch: "1.1", Digest: digest, Record: raw}
}

func TestAuditEndToEnd(t *testing.T) {
	pgPassword := os.Getenv("AUDIT_IT_PG_PASSWORD")
	if pgPassword == "" {
		t.Fatal("AUDIT_IT_PG_PASSWORD is not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, "postgres://postgres:"+url.QueryEscape(pgPassword)+"@localhost:5432/"+itDB+"?sslmode=disable")
	require.NoError(t, err)
	defer admin.Close(ctx)
	_, err = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+itSchema+" CASCADE; CREATE SCHEMA "+itSchema)
	require.NoError(t, err)

	w := sectest.New(t)
	w.GrantAll(sectest.Alice, Definitions())
	portFree(t, itPort, itSystem)
	env := w.Env(t, ID)
	for k, v := range map[string]string{"COMPONENT_ID": ID, "COMPONENT_VERSION": Version, "PORT": itPort, "STOP_DRAIN_DELAY": "50ms",
		"PG_HOST": "localhost", "PG_DATABASE": itDB, "PG_USER": "postgres", "PG_PASSWORD": pgPassword, "PG_SCHEMA": itSchema,
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
	start := func() func() {
		sctx, stop := context.WithCancel(ctx)
		done := make(chan int, 1)
		go func() { done <- launcher.Run(sctx, Definition(), opts) }()
		require.Eventually(t, func() bool {
			r, err := http.Get(itBase + "/readyz")
			if err == nil {
				r.Body.Close()
			}
			return err == nil && r.StatusCode == 200
		}, 15*time.Second, 100*time.Millisecond)
		return func() { stop(); <-done }
	}
	stop := start()

	tls := w.TLS(t, "demo/orders")
	conn, err := grpc.NewClient(itSystem, grpc.WithTransportCredentials(credentials.NewTLS(tls.ClientConfig(ID))))
	require.NoError(t, err)
	defer conn.Close()
	sender := sdkaudit.GRPCSender{Conn: conn}
	send := func(ws ...sdkaudit.Wire) []sdkaudit.Receipt {
		sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		rs, err := sender.Send(sctx, ws)
		require.NoError(t, err)
		require.Len(t, rs, len(ws))
		return rs
	}

	// three records: received in order, storage sequences 1-3, the chain over their digests
	batch := []sdkaudit.Wire{wire(t, 1, "demo.order.create"), wire(t, 2, "demo.order.confirm"), wire(t, 3, "demo.order.ship")}
	rs := send(batch...)
	chain := ""
	for i, r := range rs {
		assert.Equal(t, "received", r.Status)
		assert.Equal(t, batch[i].ID, r.ID)
		assert.Equal(t, int64(i+1), r.Seq)
		chain = chainNext(chain, batch[i].Digest)
		assert.Equal(t, chain, r.Chain)
	}
	again := send(batch...)
	assert.Equal(t, rs, again, "stored before: the original receipts")

	// another digest under a stored ID; bytes that are not the record; an ID that is not the record's
	raw, digest, err := records.Seal("be.audit.record", map[string]any{"id": batch[0].ID, "occurred_at": canon.Time(time.Now()),
		"category": "business_change", "operation": "demo.order.cancel", "outcome": "succeeded"})
	require.NoError(t, err)
	other := sdkaudit.Wire{ID: batch[0].ID, Seq: 4, Epoch: "1.1", Digest: digest, Record: raw}
	broken := wire(t, 4, "demo.order.cancel")
	broken.Digest = batch[1].Digest
	misnamed := wire(t, 4, "demo.order.cancel")
	misnamed.ID = ids.New().String()
	rs = send(other, broken, misnamed)
	assert.Equal(t, []string{"conflict", "invalid", "invalid"}, []string{rs[0].Status, rs[1].Status, rs[2].Status})

	// a gap in the source's sequence is reported and the record still stored
	gap := wire(t, 6, "demo.order.close")
	assert.Equal(t, "received", send(gap)[0].Status)
	assert.Contains(t, logs.String(), "a gap in a source's audit records")

	// nothing is backed up yet: nothing may be deleted
	wm, err := sender.Watermark(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), wm.Seq)
	assert.False(t, wm.BackedUpInEpoch)

	// the audit component stops and comes back: the source sends what it held, and the earlier records stay as they were
	stop()
	sctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	_, err = sender.Send(sctx, []sdkaudit.Wire{wire(t, 7, "demo.order.archive")})
	cancel()
	assert.Error(t, err, "the audit component is down")
	stop = start()
	defer stop()
	held := wire(t, 7, "demo.order.archive")
	require.Eventually(t, func() bool { // the source's transfer job retries every 5 s; the channel reconnects
		sctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		rs, err = sender.Send(sctx, []sdkaudit.Wire{held, batch[0]})
		return err == nil
	}, 15*time.Second, 200*time.Millisecond)
	assert.Equal(t, "received", rs[0].Status)
	assert.Equal(t, int64(5), rs[0].Seq, "the storage sequence continues")
	assert.Equal(t, int64(1), rs[1].Seq, "an earlier record keeps its receipt")

	// the query, guarded by the authorization view
	req, _ := http.NewRequest("GET", itBase+"/api/audit/records?source=demo/orders&operation=demo.order.create", nil)
	req.Header.Set("Authorization", w.Bearer(sectest.Alice))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, string(body))
	var out struct {
		Records []struct {
			Source string
			Record map[string]any
		}
	}
	require.NoError(t, json.Unmarshal(body, &out))
	require.Len(t, out.Records, 1)
	assert.Equal(t, "demo/orders", out.Records[0].Source)
	assert.Equal(t, "demo.order.create", out.Records[0].Record["operation"])
	req.Header.Set("Authorization", w.Bearer(sectest.Bob))
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode, "Bob holds no grant to read the audit records: not visible (P8.16)")
}

// portFree fails at once when a port the member needs is taken: a running cell (host ports 18xxx) or another member
// would answer in its place, and the test would assert against another process.
func portFree(t *testing.T, addrs ...string) {
	for _, a := range addrs {
		if !strings.Contains(a, ":") {
			a = "localhost:" + a
		}
		ln, err := net.Listen("tcp", a)
		require.NoError(t, err, "port %s is taken: is a cell or another member running?", a)
		_ = ln.Close()
	}
}
