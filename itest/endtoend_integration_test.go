//go:build integration

package itest

// The audit component against PostgreSQL over its real surfaces: a source component's records sent over mTLS with
// the SDK's own sender, the receipts, the watermark, a stop and restart, and the protected query (be-sdk-go's
// sectest gives the certificates, tokens and the authorization member the query is guarded by). Needs
// AUDIT_IT_PG_PASSWORD (the postgres superuser of be-postgres); missing, it fails. The member listens on 28206 (user)
// and 9206 (system). Run: go test -tags integration ./itest/ (make test-integration); the index of the steps is itest/AGENTS.md

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	audit "github.com/brickKit/infra-audit/v3/audit"

	core "github.com/brickKit/infra-audit/v3/internal/core"

	sdkaudit "github.com/brickKit/be-sdk-go/v3/audit"
	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/canon"
	"github.com/brickKit/be-sdk-go/v3/config"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/be-sdk-go/v3/records"
	"github.com/brickKit/be-sdk-go/v3/sectest"
	"github.com/jackc/pgx/v5"
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

// fixture is the running member, the source's sender and what one step leaves for the next.
type fixture struct {
	t      *testing.T
	ctx    context.Context
	admin  *pgx.Conn
	w      *sectest.World
	logs   *bytes.Buffer
	start  func() func() // starts the member, answers its stop
	stop   func()        // stops the running one
	sender sdkaudit.GRPCSender
	batch  []sdkaudit.Wire // the first three records (receive)
}

func TestAuditEndToEnd(t *testing.T) {
	fx := start(t)
	fx.receive()
	fx.recordService()
	fx.refusals()
	fx.restart()
	fx.query()
	fx.chainCheck()
}

// send sends the records with the source's sender and answers their receipts.
func (fx *fixture) send(ws ...sdkaudit.Wire) []sdkaudit.Receipt {
	sctx, cancel := context.WithTimeout(fx.ctx, 5*time.Second)
	defer cancel()
	rs, err := fx.sender.Send(sctx, ws)
	require.NoError(fx.t, err)
	require.Len(fx.t, rs, len(ws))
	return rs
}

// start makes the world, migrates and runs the member, and connects as the source demo/orders.
func start(t *testing.T) *fixture {
	pgPassword := os.Getenv("AUDIT_IT_PG_PASSWORD")
	if pgPassword == "" {
		t.Fatal("AUDIT_IT_PG_PASSWORD is not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, "postgres://postgres:"+url.QueryEscape(pgPassword)+"@localhost:5432/"+itDB+"?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() { admin.Close(ctx) })
	_, err = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+itSchema+" CASCADE; CREATE SCHEMA "+itSchema)
	require.NoError(t, err)

	w := sectest.New(t)
	w.GrantAll(sectest.Alice, core.Definitions())
	// Bob may read the audit records' source and ID only (AU13: the fields cut to the caller's grant)
	w.Grant(sectest.Bob, sdkauthz.Segment{ResourceType: core.Records, Operation: core.OpRead, Atoms: map[string]sdkauthz.Atom{},
		ReadFields: []string{"id", "source"}, Valid: sdkauthz.Window{From: "2026-01-01T00:00:00.000000Z"}})
	portFree(t, itPort, itSystem)
	env := w.Env(t, core.ID)
	for k, v := range map[string]string{"COMPONENT_ID": core.ID, "COMPONENT_VERSION": audit.Version, "PORT": itPort, "STOP_DRAIN_DELAY": "50ms",
		"PG_HOST": "localhost", "PG_DATABASE": itDB, "PG_USER": "postgres", "PG_PASSWORD": pgPassword, "PG_SCHEMA": itSchema,
		"PG_OWNER_USER": "postgres", "PG_OWNER_PASSWORD": pgPassword, "AUDIT_CHAIN_CHECK_INTERVAL": "1s"} {
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
	require.Equal(t, launcher.ExitOK, launcher.Run(ctx, audit.Definition(), migrate))
	start := func() func() {
		sctx, stop := context.WithCancel(ctx)
		done := make(chan int, 1)
		go func() { done <- launcher.Run(sctx, audit.Definition(), opts) }()
		require.Eventually(t, func() bool {
			r, err := http.Get(itBase + "/readyz")
			if err == nil {
				r.Body.Close()
			}
			return err == nil && r.StatusCode == 200
		}, 15*time.Second, 100*time.Millisecond)
		return func() { stop(); <-done }
	}
	fx := &fixture{t: t, ctx: ctx, admin: admin, w: w, logs: &logs, start: start}
	fx.stop = start()
	t.Cleanup(func() { fx.stop() }) // the one running at the end (restart starts it again)

	tls := w.TLS(t, "demo/orders")
	conn, err := grpc.NewClient(itSystem, grpc.WithTransportCredentials(credentials.NewTLS(tls.ClientConfig(core.ID))))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	fx.sender = sdkaudit.GRPCSender{Conn: conn}
	return fx
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
