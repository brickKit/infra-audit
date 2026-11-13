package audit

import (
	"embed"
	"io/fs"

	archive "github.com/brickKit/infra-audit/v3/internal/archive"
	core "github.com/brickKit/infra-audit/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/component"
	"github.com/brickKit/be-sdk-go/v3/config"
	"github.com/brickKit/be-sdk-go/v3/runtime"
	"github.com/brickKit/be-sdk-go/v3/tiering"
)

// The component's version and its system port.
const (
	Version  = "3.0.45"
	GRPCPort = 9206
)

// Keys are the component's own configuration keys.
func Keys() []config.Key {
	return []config.Key{
		// the components that may list the evidence (P12.13) and the audit records (RecordService), comma-separated component IDs; by default the recovery
		// tool's service identity (be-ops recover, A136)
		{Name: "AUDIT_EVIDENCE_READERS", Type: config.String, Default: core.DefaultEvidenceReader},
		// how often every source's chain is verified (AU14: a record changed or taken out is an integrity event)
		{Name: "AUDIT_CHAIN_CHECK_INTERVAL", Type: config.Duration, Default: core.DefaultChainCheck},
	}
}

//go:embed migrations/*.sql
var migrationFiles embed.FS

func migrations() fs.FS {
	sub, _ := fs.Sub(migrationFiles, "migrations")
	return sub
}

// Definition is the component as the launcher and the shells take it.
func Definition() component.Definition {
	return component.Definition{ID: core.ID, Version: Version, Keys: Keys(), Migrations: migrations(), GRPCPort: GRPCPort, Authz: core.Definitions(),
		Recovery: core.OpRecover, Replay: core.OpReplay, Tiering: []tiering.Class{archive.RecordsClass()},
		New: func(rt *runtime.Runtime) (component.Module, error) {
			m := &Module{b: &core.Base{RT: rt}}
			if rt.Tiering != nil { // the history of the records, archived ones included (P15.26)
				if err := rt.Tiering.Register(archive.RecordClass, archive.Reader(m.b)); err != nil {
					return nil, err
				}
			}
			return m, nil
		}}
}
