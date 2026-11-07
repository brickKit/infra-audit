//go:build integration

package itest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	audit "github.com/brickKit/infra-audit/v3/audit"

	archive "github.com/brickKit/infra-audit/v3/internal/archive"
	core "github.com/brickKit/infra-audit/v3/internal/core"

	sdkaudit "github.com/brickKit/be-sdk-go/v3/audit"
	"github.com/brickKit/be-sdk-go/v3/canon"
	"github.com/brickKit/be-sdk-go/v3/component"
	"github.com/brickKit/be-sdk-go/v3/config"
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/be-sdk-go/v3/runtime"
	"github.com/brickKit/be-sdk-go/v3/sectest"
	"github.com/brickKit/be-sdk-go/v3/tiering"
	auditv3 "github.com/brickKit/infra-audit/contract/v3/gen/go/be/audit/v3"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// The audit records as a data class (assembly protocol 11, protocol 03's AU17, decision A158): records received
// long ago are sealed, copied and released from the online table; the same records sent again get their first
// receipts from the archive; the chains still verify; the recovery's list refuses a range into the archive; the
// query's history reads them; a legal hold keeps them past their retention, and they go once it is lifted.
func TestTheAuditRecordsTier(t *testing.T) {
	pgPassword := os.Getenv("AUDIT_IT_PG_PASSWORD")
	require.NotEmpty(t, pgPassword, "AUDIT_IT_PG_PASSWORD")
	ctx := context.Background()
	const schema = "audit_tier_it"
	admin, err := pgx.Connect(ctx, "postgres://postgres:"+url.QueryEscape(pgPassword)+"@localhost:5432/"+itDB+
		"?sslmode=disable&search_path="+schema)
	require.NoError(t, err)
	defer admin.Close(ctx)
	_, err = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE; CREATE SCHEMA "+schema)
	require.NoError(t, err)

	w := sectest.New(t)
	w.GrantAll(sectest.Alice, core.Definitions())
	portFree(t, itPort, itSystem)
	env := w.Env(t, core.ID)
	for k, v := range map[string]string{"COMPONENT_ID": core.ID, "COMPONENT_VERSION": audit.Version, "PORT": itPort, "STOP_DRAIN_DELAY": "50ms",
		"PG_HOST": "localhost", "PG_DATABASE": itDB, "PG_USER": "postgres", "PG_PASSWORD": pgPassword, "PG_SCHEMA": schema,
		"PG_OWNER_USER": "postgres", "PG_OWNER_PASSWORD": pgPassword, "AUDIT_CHAIN_CHECK_INTERVAL": "1s",
		tiering.RetentionKey(archive.RecordClass): "365/3650/the customer's audit retention, region ES"} {
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
	def := audit.Definition()
	built := make(chan *runtime.Runtime, 1)
	newModule := def.New
	def.New = func(rt *runtime.Runtime) (component.Module, error) { built <- rt; return newModule(rt) }
	migrate := opts
	migrate.Args = []string{"migrate"}
	require.Equal(t, launcher.ExitOK, launcher.Run(ctx, def, migrate))
	sctx, stop := context.WithCancel(ctx)
	done := make(chan int, 1)
	go func() { done <- launcher.Run(sctx, def, opts) }()
	defer func() { stop(); <-done }()
	require.Eventually(t, func() bool {
		r, err := http.Get(itBase + "/readyz")
		if err == nil {
			r.Body.Close()
		}
		return err == nil && r.StatusCode == 200
	}, 15*time.Second, 100*time.Millisecond)
	rt := <-built
	cat := rt.Tiering
	require.NotNil(t, cat)
	assert.Empty(t, cat.PendingRetentions(), "the retention given at deployment")
	cat.Grace, cat.Backups = 0, func(context.Context) (time.Time, bool, error) { return time.Now().Add(time.Hour), true, nil }

	conn, err := grpc.NewClient(itSystem, grpc.WithTransportCredentials(credentials.NewTLS(w.TLS(t, "demo/orders").ClientConfig(core.ID))))
	require.NoError(t, err)
	defer conn.Close()
	sender := sdkaudit.GRPCSender{Conn: conn}
	send := func(ws ...sdkaudit.Wire) []sdkaudit.Receipt {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		rs, err := sender.Send(c, ws)
		require.NoError(t, err)
		return rs
	}
	old := []sdkaudit.Wire{wire(t, 1, "demo.order.create"), wire(t, 2, "demo.order.confirm")}
	first := send(old...)
	_, err = admin.Exec(ctx, "UPDATE audit_record SET received_at = now() - interval '800 days'")
	require.NoError(t, err)

	groups, err := cat.Groupings(ctx, archive.RecordClass)
	require.NoError(t, err)
	require.Len(t, groups, 1, "one source, one month")
	sealed, err := cat.Seal(ctx, archive.RecordClass, groups[0])
	require.NoError(t, err)
	_, err = cat.Transfer(ctx, sealed.Unit)
	require.NoError(t, err)
	why, err := cat.Release(ctx, sealed.Unit)
	require.NoError(t, err)
	require.Empty(t, why)
	var online int
	require.NoError(t, admin.QueryRow(ctx, "SELECT count(*) FROM audit_record").Scan(&online))
	require.Zero(t, online, "the online rows are released")

	// every record of the source released, none online since (3.0.32; the real cell T5-S found 3.0.31 reporting a break
	// here): the head is the archive's last record's chain; a head that is not is a break
	breaks := func() int {
		var n int
		require.NoError(t, admin.QueryRow(ctx, "SELECT count(*) FROM audit_chain_break").Scan(&n))
		return n
	}
	time.Sleep(2500 * time.Millisecond) // the chain check runs every second
	require.Zero(t, breaks(), "every record released: the chain ends in the archive")
	var head string
	require.NoError(t, admin.QueryRow(ctx, "SELECT chain FROM audit_source WHERE source = 'demo/orders'").Scan(&head))
	_, err = admin.Exec(ctx, "UPDATE audit_source SET chain = 'sha256:' || repeat('0', 64) WHERE source = 'demo/orders'")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return breaks() == 1 }, 5*time.Second, 100*time.Millisecond,
		"a head the archive's last record does not give is a break")
	_, err = admin.Exec(ctx, "UPDATE audit_source SET chain = $1 WHERE source = 'demo/orders'", head)
	require.NoError(t, err)
	_, err = admin.Exec(ctx, "DELETE FROM audit_chain_break")
	require.NoError(t, err)

	assert.Equal(t, first, send(old...), "sent again: the first receipts, from the archive")
	later := wire(t, 3, "demo.order.ship")
	assert.Equal(t, "received", send(later)[0].Status)
	time.Sleep(2500 * time.Millisecond) // the chain check runs every second
	assert.Zero(t, breaks(), "the chain goes on from the first online record")

	list, err := grpc.NewClient(itSystem, grpc.WithTransportCredentials(credentials.NewTLS(w.TLS(t, core.DefaultEvidenceReader).ClientConfig(core.ID))))
	require.NoError(t, err)
	defer list.Close()
	_, err = auditv3.NewRecordServiceClient(list).List(ctx, &auditv3.RecordServiceListRequest{Since: canon.Time(time.Now().Add(-900 * 24 * time.Hour))})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err), "a range into the archive: never a part (%v)", err)
	page, err := auditv3.NewRecordServiceClient(list).List(ctx, &auditv3.RecordServiceListRequest{Since: canon.Time(time.Now().Add(-time.Hour))})
	require.NoError(t, err)
	assert.Len(t, page.GetRecords(), 1, "a recent range: the online records")

	call := func(method, path, body string) (int, string) {
		req, _ := http.NewRequest(method, itBase+path, strings.NewReader(body))
		req.Header.Set("Authorization", w.Bearer(sectest.Alice))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "test")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	code, body := call("GET", "/api/audit/records?history=true&source=demo/orders", "")
	require.Equal(t, http.StatusOK, code, body)
	var hist struct{ Records []map[string]any }
	require.NoError(t, json.Unmarshal([]byte(body), &hist))
	assert.Len(t, hist.Records, 3, "two archived and one online")
	code, body = call("GET", "/api/audit/records?history=true&operation=demo.order.confirm", "")
	require.Equal(t, http.StatusOK, code, body)
	require.NoError(t, json.Unmarshal([]byte(body), &hist))
	require.Len(t, hist.Records, 1, "an archived record selected by its operation")
	assert.Equal(t, old[1].ID, hist.Records[0]["id"])

	// AU17: the retention has run and the deletion is approved, but a legal hold keeps the records
	_, err = admin.Exec(ctx, "UPDATE be_tier_unit SET anchor_max = now() - interval '400 days' WHERE id = $1", sealed.Unit)
	require.NoError(t, err)
	code, body = call("POST", "/be/tiering/"+archive.RecordClass+"/approvals", `{"scope_kind": "class", "scope": "`+archive.RecordClass+
		`", "reason": "the retention has run"}`)
	require.Equal(t, http.StatusCreated, code, body)
	code, body = call("POST", "/be/tiering/"+archive.RecordClass+"/holds", `{"scope_kind": "record", "scope": "`+
		archive.RecordClass+"/demo/orders/"+old[0].ID+`", "source": "legal", "reason": "a court order"}`)
	require.Equal(t, http.StatusCreated, code, body)
	var hold struct{ ID string }
	require.NoError(t, json.Unmarshal([]byte(body), &hold))
	why, err = cat.Destroy(ctx, sealed.Unit)
	require.NoError(t, err)
	assert.Equal(t, []string{tiering.WhyHeld}, why, "AU17: held, not deleted")
	code, body = call("POST", "/be/tiering/"+archive.RecordClass+"/holds/"+hold.ID+"/lift", "")
	require.Equal(t, http.StatusNoContent, code, body)
	why, err = cat.Destroy(ctx, sealed.Unit)
	require.NoError(t, err)
	assert.Empty(t, why, "the hold lifted: deleted at the end of the retention")
	code, body = call("GET", "/api/audit/records?history=true&source=demo/orders", "")
	require.Equal(t, http.StatusOK, code, body)
	require.NoError(t, json.Unmarshal([]byte(body), &hist))
	assert.Len(t, hist.Records, 1, "the destroyed records are gone")
}
