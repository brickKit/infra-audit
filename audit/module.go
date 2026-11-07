package audit

import (
	"context"
	"strings"

	"github.com/brickKit/be-sdk-go/v3/grpcx"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/work"
	auditv3 "github.com/brickKit/infra-audit/contract/v3/gen/go/be/audit/v3"
	chain "github.com/brickKit/infra-audit/v3/internal/chain"
	core "github.com/brickKit/infra-audit/v3/internal/core"
	reading "github.com/brickKit/infra-audit/v3/internal/reading"
)

// Module is the audit component as the launcher and the shells run it: it only assembles the business modules under
// internal/ (their map is internal/AGENTS.md); the gRPC services answer through them (services.go).
type Module struct {
	auditv3.UnimplementedIntakeServiceServer
	auditv3.UnimplementedEvidenceServiceServer
	b *core.Base
}

// Services registers the intake; every component of the deployment sends its records.
func (m *Module) Services(s *grpcx.Services) error {
	s.Register(&auditv3.IntakeService_ServiceDesc, m, map[string]grpcx.Method{
		"Submit":    {Operation: core.OpSubmit, Callers: []string{grpcx.AnyComponent}},
		"Watermark": {Operation: core.OpWatermark, Callers: []string{grpcx.AnyComponent}}})
	var readers []string
	for _, r := range strings.Split(m.b.RT.Config.String("AUDIT_EVIDENCE_READERS"), ",") {
		if r = strings.TrimSpace(r); r != "" {
			readers = append(readers, r)
		}
	}
	s.Register(&auditv3.EvidenceService_ServiceDesc, m, map[string]grpcx.Method{
		"Append": {Operation: core.OpAppend, Callers: []string{grpcx.AnyComponent}},
		"List":   {Operation: core.OpList, Callers: readers}})
	// the audit records for the same readers (a full recovery's loss range, contract alpha.3)
	s.Register(&auditv3.RecordService_ServiceDesc, reading.RecordList{M: m.b}, map[string]grpcx.Method{"List": {Operation: core.OpListRecords, Callers: readers}})
	return nil
}

// Routes registers the query.
func (m *Module) Routes(r *httpx.Routes) error {
	r.Add(httpx.Route{Method: "GET", Path: "/api/audit/records", Operation: core.OpRead,
		Handle: func(c *httpx.Context) error { return reading.Query(m.b, c) }})
	return nil
}

// Jobs declares the chain check (AU14): every record is read each time, at the configured interval.
func (m *Module) Jobs(j *work.Jobs) error {
	j.Add(work.Job{Name: chain.JobChainCheck, Kind: work.Singleton, Effect: work.Local, MaxAttempts: work.Unlimited,
		Interval: m.b.RT.Config.Duration("AUDIT_CHAIN_CHECK_INTERVAL"),
		Handle:   func(ctx context.Context, r *work.Run) error { return chain.ChainCheck(m.b, ctx, r) }})
	return nil
}
