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
# Windows release mode needs a windows host, so it uses package mode.

[doc('Package the app for darwin, linux and windows (linux/windows cross-build via Docker)')]
@package-all: package
    go tool fyne-cross linux -arch amd64 -release -env GOTOOLCHAIN=auto
    go tool fyne-cross windows -arch amd64 -env GOTOOLCHAIN=auto
