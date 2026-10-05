.PHONY: build test vet fixtures demo validation release

build:
	mkdir -p bin
	go build -trimpath -o bin/protocarry ./cmd/protocarry
	go build -trimpath -o bin/demo-adapter ./cmd/demo-adapter

test:
	go test -race ./...

vet:
	go vet ./...

fixtures:
	go run ./cmd/fixtures

demo: build
	mkdir -p evidence
	./bin/protocarry check -config examples/demo/preserve.json -out evidence/demo-preserve
	./bin/protocarry check -config examples/demo/copy.json -out evidence/demo-copy; test $$? -eq 1
	./bin/protocarry check -config examples/demo/json.json -out evidence/demo-json; test $$? -eq 1
	./bin/protocarry check -config examples/demo/mutate.json -out evidence/demo-mutate

validation: build
	cd validation/protobufjs && npm ci --ignore-scripts --no-audit --no-fund
	cd validation/protobufjs && npm test
	cd validation/protobufjs && npm run validate

release:
	sh scripts/release.sh
