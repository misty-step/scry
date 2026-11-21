PORT ?= 8080

.PHONY: run test build vet verify-formal eval docker clean

# Start Scry on PORT in test mode, with seeded demo data. No keys needed.
run:
	SCRY_MODE=test SCRY_SEED_DEMO=1 SCRY_DB=$${SCRY_DB:-data/demo.db} PORT=$(PORT) go run ./cmd/scry

test:
	go vet ./...
	go test ./...

BUILD_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.buildSHA=$(BUILD_SHA)" -o bin/scry ./cmd/scry
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/scry-watch ./cmd/scry-watch

vet:
	go vet ./...

# Quint specs (typecheck, tests, Rust-backend simulation), the rapid state
# machines and transition tables (-race, then deeper), and fuzz smoke runs.
# VERIFY_APALACHE=1 adds bounded Apalache model checking (slow).
verify-formal:
	./scripts/verify-formal.sh

# Live 4-surface micro-eval bench on OpenRouter (grading, explainer, speech,
# drafting). Spends real money from OPENROUTER_API_KEY, capped by
# EVAL_MAX_SPEND; see evals/README.md. Examples:
#   make eval
#   make eval EVAL_ARGS="--surface grading --limit 3"
EVAL_OUT ?= evals/results/latest
EVAL_MAX_SPEND ?= 1.35
EVAL_ARGS ?= --surface all
eval:
	go run ./evals/cmd/scry-eval --out $(EVAL_OUT) --max-spend $(EVAL_MAX_SPEND) $(EVAL_ARGS)

docker:
	docker build --build-arg BUILD_SHA=$(BUILD_SHA) -t scry .

clean:
	rm -rf bin data
