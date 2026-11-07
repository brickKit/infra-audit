package audit

import (
	"context"

	auditv3 "github.com/brickKit/infra-audit/contract/v3/gen/go/be/audit/v3"
	evidence "github.com/brickKit/infra-audit/v3/internal/evidence"
	intake "github.com/brickKit/infra-audit/v3/internal/intake"
)

// The gRPC methods of the system surface answer through internal/intake and internal/evidence (RecordService is
// internal/reading's RecordList, registered in module.go).

// Submit is IntakeService.Submit.
func (m *Module) Submit(ctx context.Context, in *auditv3.SubmitRequest) (*auditv3.SubmitResponse, error) {
	return intake.Submit(m.b, ctx, in)
}

// Watermark is IntakeService.Watermark.
func (m *Module) Watermark(ctx context.Context, in *auditv3.WatermarkRequest) (*auditv3.WatermarkResponse, error) {
	return intake.Watermark(m.b, ctx, in)
}

// Append is EvidenceService.Append.
func (m *Module) Append(ctx context.Context, in *auditv3.AppendRequest) (*auditv3.AppendResponse, error) {
	return evidence.Append(m.b, ctx, in)
}

// List is EvidenceService.List.
func (m *Module) List(ctx context.Context, in *auditv3.ListRequest) (*auditv3.ListResponse, error) {
	return evidence.List(m.b, ctx, in)
}
