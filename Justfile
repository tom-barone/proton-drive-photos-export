[private]
default:
    just --list

[doc('Run all precommit checks')]
precommit: install format lint test build

# === Development ===

[doc('Install dependencies')]
install:
    go mod download

[doc('Format code and apply fixes')]
format:
    gofmt -w .
    just --fmt --unstable

[doc('Lint code')]
lint:
    go vet ./...
    test -z "$(gofmt -l .)" || (gofmt -l . && exit 1)
    hadolint Dockerfile
    actionlint -color

[doc('Run tests')]
test:
    go test ./...

[doc('Run the app')]
run:
    go run .

[doc('Generate app screenshots into screenshots/')]
screenshots:
    SCREENSHOTS_DIR="{{ justfile_directory() }}/screenshots" go test ./adapters/in/fynegui -run '^TestScreenshots$' -count=1

[doc('Build for the current platform')]
build:
    go build -o dist/photos-export .

[doc('Clean build and packaging artifacts')]
clean:
    rm -rf dist

# === Build ===

[doc('Package the app for darwin, linux and windows into dist (needs Docker)')]
package:
    docker build --output dist .
