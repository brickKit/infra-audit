// Command infra-audit runs the audit component alone (serve, migrate); the shells compile in member.Definition().
package main

import (
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/infra-audit/v3/member"
)

func main() { launcher.Main(member.Definition()) }
