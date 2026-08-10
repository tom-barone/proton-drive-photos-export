[private]
@default:
    just --list

[doc('Run all precommit checks')]
@precommit: install format lint test

# === Development ===

[doc('Install dependencies')]
@install:
    go mod download

[doc('Format code and apply fixes')]
@format:
    gofmt -w .
    just --fmt --unstable

[doc('Lint code')]
@lint:
    go vet ./...
    test -z "$(gofmt -l .)" || (gofmt -l . && exit 1)

[doc('Run tests')]
@test:
    go test ./...

[doc('Run the app')]
@run:
    go run .

# === Build ===

[doc('Build for the current platform')]
@build:
    go build -o photo-export .

[doc('Package the app for the current platform')]
@package:
    go tool fyne package --release

# GOTOOLCHAIN=auto lets the containers fetch the Go version go.mod
# requires; the fyne-cross images pin an older one with GOTOOLCHAIN=local.
# Windows uses package mode because fyne's release mode for windows only
# runs on windows hosts. The PATH shim lets fyne-cross find the
# go.mod-pinned fyne CLI when building darwin on the host toolchain.

[doc('Package the app for darwin, linux and windows into fyne-cross/dist (needs Docker)')]
package-all:
    #!/usr/bin/env bash
    set -euo pipefail
    shim="$(mktemp -d)"
    printf '#!/bin/sh\nexec go tool fyne "$@"\n' > "$shim/fyne"
    chmod +x "$shim/fyne"
    export PATH="$shim:$PATH"
    go tool fyne-cross darwin -arch amd64,arm64 -env GOTOOLCHAIN=auto
    go tool fyne-cross linux -arch amd64 -release -env GOTOOLCHAIN=auto
    go tool fyne-cross windows -arch amd64 -env GOTOOLCHAIN=auto
