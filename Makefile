PORT ?= 8080

.PHONY: run test build vet docker clean

# Start Scry on PORT in test mode, with seeded demo data. No keys needed.
run:
	SCRY_MODE=test SCRY_SEED_DEMO=1 SCRY_DB=$${SCRY_DB:-data/demo.db} PORT=$(PORT) go run ./cmd/scry

test:
	go vet ./...
	go test ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/scry ./cmd/scry

vet:
	go vet ./...

docker:
	docker build -t scry .

clean:
	rm -rf bin data
