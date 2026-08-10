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
