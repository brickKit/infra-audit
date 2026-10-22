// Command infra-audit runs the audit component alone (serve, migrate); the shells compile in audit.Definition().
package main

import (
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/infra-audit/v3/audit"
)

func main() { launcher.Main(audit.Definition()) }
