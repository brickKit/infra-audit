//go:build integration

package itest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/brickKit/be-sdk-go/v3/canon"
	"github.com/brickKit/be-sdk-go/v3/sectest"
	auditv3 "github.com/brickKit/infra-audit/contract/v3/gen/go/be/audit/v3"
	core "github.com/brickKit/infra-audit/v3/internal/core"
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// recordService: RecordService.List (contract alpha.3) for the configured readers only; since, categories, pages by
// position.
func (fx *fixture) recordService() {
	t, ctx := fx.t, fx.ctx
	// RecordService.List (contract alpha.3): the configured readers only; since, categories, pages by position
	listAs := func(as string, req *auditv3.RecordServiceListRequest) (*auditv3.RecordServiceListResponse, error) {
		c, err := grpc.NewClient(itSystem, grpc.WithTransportCredentials(credentials.NewTLS(fx.w.TLS(t, as).ClientConfig(core.ID))))
		require.NoError(t, err)
		defer c.Close()
		lctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return auditv3.NewRecordServiceClient(c).List(lctx, req)
	}
	minuteAgo := canon.Time(time.Now().Add(-time.Minute))
	_, err := listAs("demo/orders", &auditv3.RecordServiceListRequest{Since: minuteAgo})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "a source is no reader: %v", err)
	page, err := listAs(core.DefaultEvidenceReader, &auditv3.RecordServiceListRequest{Since: minuteAgo, Categories: []string{"business_change"}, Limit: 2})
	require.NoError(t, err)
	require.Len(t, page.GetRecords(), 2)
	assert.True(t, page.GetMore())
	assert.Equal(t, "demo/orders", page.GetRecords()[0].GetSource())
	assert.Equal(t, []int64{1, 2}, []int64{page.GetRecords()[0].GetSourceSeq(), page.GetRecords()[1].GetSourceSeq()})
	assert.Equal(t, "1.1", page.GetRecords()[0].GetSourceEpoch())
	assert.Equal(t, fx.batch[0].Record, page.GetRecords()[0].GetRecord(), "the bytes as received")
	page, err = listAs(core.DefaultEvidenceReader, &auditv3.RecordServiceListRequest{Since: minuteAgo, After: page.GetRecords()[1].GetPosition()})
	require.NoError(t, err)
	require.Len(t, page.GetRecords(), 1)
	assert.False(t, page.GetMore())
	assert.EqualValues(t, 3, page.GetRecords()[0].GetSourceSeq())
	for _, req := range []*auditv3.RecordServiceListRequest{{Since: canon.Time(time.Now().Add(time.Minute))},
		{Since: minuteAgo, Categories: []string{"sensitive_read"}}} {
		page, err = listAs(core.DefaultEvidenceReader, req)
		require.NoError(t, err)
		assert.Empty(t, page.GetRecords(), "%v", req)
	}
	_, err = listAs(core.DefaultEvidenceReader, &auditv3.RecordServiceListRequest{Since: "yesterday"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// query: the user surface, guarded by the authorization view; the query itself is audited, an unknown caller sees
// nothing, Bob's fields are cut to his grant (AU13).
func (fx *fixture) query() {
	t, ctx := fx.t, fx.ctx
	// the query, guarded by the authorization view
	req, _ := http.NewRequest("GET", itBase+"/api/audit/records?source=demo/orders&operation=demo.order.create", nil)
	req.Header.Set("Authorization", fx.w.Bearer(sectest.Alice))
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
	queries := func() (n int) {
		require.NoError(t, fx.admin.QueryRow(ctx, "SELECT count(*) FROM "+itSchema+".be_audit_log WHERE category = 'sensitive_read' "+
			"AND operation = $1", core.OpRead).Scan(&n))
		return n
	}
	assert.Equal(t, 1, queries(), "AU13: the query itself is audited")
	req.Header.Set("Authorization", fx.w.Bearer("0190f3a1-0000-7000-8000-0000000000c3"))
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode, "no grant to read the audit records: not visible (P8.16)")
	req.Header.Set("Authorization", fx.w.Bearer(sectest.Bob))
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, string(body))
	var cut struct {
		Records []map[string]any `json:"records"`
	}
	require.NoError(t, json.Unmarshal(body, &cut))
	require.Len(t, cut.Records, 1)
	assert.Equal(t, "demo/orders", cut.Records[0]["source"])
	assert.Nil(t, cut.Records[0]["record"], "AU13: the fields cut to Bob's grant")
	assert.Equal(t, []any{"received_at", "record"}, cut.Records[0]["_masked"])
	assert.Equal(t, 2, queries())
}
