default: test

test:
    go test -race ./...

test-386:
    GOARCH=386 go test ./...

test-wasm:
    GOOS=js GOARCH=wasm go test -exec="env -i $(command -v node) $(go env GOROOT)/lib/wasm/wasm_exec_node.js" ./...

fmt:
    gofmt -w pcm.go pcm_test.go cmd

lint:
    go vet ./...
    GOOS=js GOARCH=wasm go vet ./...

fuzz:
    go test -run '^$' -fuzz '^FuzzPCMReader$' -fuzztime=10s -parallel=2

ci: test test-386 test-wasm lint

check-deps:
    ./scripts/release-guard.sh deps

check-unreleased:
    ./scripts/release-guard.sh unreleased

release-check VERSION:
    ./scripts/release-guard.sh gate {{VERSION}}

tag-release VERSION:
    ./scripts/release-guard.sh tag {{VERSION}}
