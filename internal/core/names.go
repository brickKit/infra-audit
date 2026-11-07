package core

import (
	"time"

	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
)

// Identity of the component, its resources and operations.
const (
	ID          = "infra/audit"
	Records     = "infra.audit.record"
	OpRead      = "infra.audit.record.read"
	Intake      = "infra.audit.intake"
	OpSubmit    = "infra.audit.intake.submit"
	OpWatermark = "infra.audit.intake.watermark"
	IntakeRead  = "infra.audit.intake.read"
	Evidence    = "infra.audit.evidence"
	OpAppend    = "infra.audit.evidence.append"
	OpList      = "infra.audit.evidence.list"
	System      = "infra.audit.system"
	OpSystem    = "infra.audit.system.read"
	// OpRecover is a full recovery's checks (be-protocol chapter 14: the recovery tool only).
	OpRecover = "infra.audit.system.recover"
	// OpReplay is the evidence replay (be.evidence.v1.ReplayService): the destructions of audit records after a full
	// recovery's point are done again (be-protocol P15.30).
	OpReplay = "infra.audit.system.replay"
	// OpListRecords is RecordService.List: the configured readers read the audit records (a full recovery's step 9).
	OpListRecords = "infra.audit.record.list"
	MaxBatch      = 500
	// DefaultChainCheck is how often the chains are verified by default (AU14): every record is read each time.
	DefaultChainCheck = 24 * time.Hour
	// DefaultEvidenceReader is the recovery tool's service identity.
	DefaultEvidenceReader = "ops/recover"
)

// The audit records' holds and deletion approvals (assembly protocol 11), declared on the records resource.
const (
	OpHold          = "infra.audit.record.hold"
	OpApproveDelete = "infra.audit.record.approve_destruction"
)

// Definitions are the component's declarations.
func Definitions() *sdkauthz.Definitions {
	none := map[string]sdkauthz.Dimension{}
	return &sdkauthz.Definitions{Component: ID,
		Resources: []sdkauthz.Resource{
			{Type: Records, ViewOperation: OpRead, Dimensions: none, Fields: map[string]sdkauthz.Field{
				"source": {Class: sdkauthz.Ordinary}, "id": {Class: sdkauthz.Ordinary}, "record": {Class: sdkauthz.Ordinary},
				"received_at": {Class: sdkauthz.Ordinary}}},
			{Type: Intake, ViewOperation: IntakeRead, Dimensions: none, Fields: map[string]sdkauthz.Field{}},
			{Type: Evidence, ViewOperation: OpList, Dimensions: none, Fields: map[string]sdkauthz.Field{}},
			{Type: System, ViewOperation: OpSystem, Dimensions: none, Fields: map[string]sdkauthz.Field{}}},
		Operations: []sdkauthz.Operation{
			{Key: OpRead, ResourceType: Records, Purpose: "special_read", RiskClass: "low", Audit: true}, // AU13
			{Key: OpListRecords, ResourceType: Records, Purpose: "read", RiskClass: "low"},
			{Key: IntakeRead, ResourceType: Intake, Purpose: "read", RiskClass: "low"},
			{Key: OpSubmit, ResourceType: Intake, Purpose: "action", RiskClass: "low", SideEffects: []string{"business_write"}},
			{Key: OpWatermark, ResourceType: Intake, Purpose: "read", RiskClass: "low"},
			{Key: OpAppend, ResourceType: Evidence, Purpose: "action", RiskClass: "low", SideEffects: []string{"business_write"}},
			{Key: OpList, ResourceType: Evidence, Purpose: "read", RiskClass: "low"},
			{Key: OpSystem, ResourceType: System, Purpose: "read", RiskClass: "low"},
			{Key: OpRecover, ResourceType: System, Purpose: "read", RiskClass: "low"},
			{Key: OpReplay, ResourceType: System, Purpose: "manage", RiskClass: "high", SideEffects: []string{"business_write"}},
			// a legal hold over audit records and the approval of their deletion at the end of the retention (AU17)
			{Key: OpHold, ResourceType: Records, Purpose: "manage", RiskClass: "high", Audit: true},
			{Key: OpApproveDelete, ResourceType: Records, Purpose: "manage", RiskClass: "high", Audit: true},
		}}
}
