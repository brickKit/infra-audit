.PHONY: test test-integration lint

test:  ## unit tests: the chain digest, the evidence categories, the manifest, merge safety
	go vet ./... && go test ./...

test-integration:  ## L2 against PostgreSQL (AUDIT_IT_PG_PASSWORD) and be-sdk-go's sectest deployment; see itest/AGENTS.md
	go test -count=1 -tags integration ./itest/

lint:
	brickkit lint
