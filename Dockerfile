# Packages the app for darwin, linux and windows in one docker build:
#
#   docker build --output dist .
#
# Each target stage replicates what fyne-cross v1.6.3 does: run `fyne
# package` in a fyneio/fyne-cross-images container with a zig cross
# compiler. GOTOOLCHAIN=auto lets go fetch the version go.mod requires;
# the images pin an older one with GOTOOLCHAIN=local. The darwin stages
# pass -ldflags=-w to skip DWARF output, since the go linker would emit
# it with dsymutil, which the image does not have.

# Rely on the joseluisq/macosx-sdks mirror for darwin SDKs
FROM fyneio/fyne-cross-images:darwin AS darwin-sdk
ADD --checksum=sha256:beee7212d265a6d2867d0236cc069314b38d5fb3486a6515734e76fa210c784c https://github.com/joseluisq/macosx-sdks/releases/download/26.1/MacOSX26.1.sdk.tar.xz /
RUN mkdir /sdk && tar -xJf /MacOSX26.1.sdk.tar.xz -C /sdk --strip-components=1

# The go.mod-pinned fyne CLI, built for the build host. Built in its own
# stage because the target stages set GOOS/GOARCH, which would produce a
# fyne binary the container cannot run.
FROM fyneio/fyne-cross-images:linux AS fyne-cli
WORKDIR /app
COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache \
    go build -o /usr/local/bin/fyne fyne.io/tools/cmd/fyne

FROM fyneio/fyne-cross-images:darwin AS darwin-amd64
WORKDIR /app
COPY . .
COPY --from=fyne-cli /usr/local/bin/fyne /usr/local/bin/fyne
COPY --from=darwin-sdk /sdk /sdk
ENV GOTOOLCHAIN=auto CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
    CC="zig cc -target x86_64-macos.10.12 -isysroot /sdk -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks" \
    CXX="zig c++ -target x86_64-macos.10.12 -isysroot /sdk -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks" \
    CGO_LDFLAGS="--sysroot /sdk -F/sdk/System/Library/Frameworks -L/usr/lib" \
    GOFLAGS="-buildmode=pie -ldflags=-w"
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache \
    fyne package -os darwin && mkdir /out && mv ./*.app /out/

FROM fyneio/fyne-cross-images:darwin AS darwin-arm64
WORKDIR /app
COPY . .
COPY --from=fyne-cli /usr/local/bin/fyne /usr/local/bin/fyne
COPY --from=darwin-sdk /sdk /sdk
ENV GOTOOLCHAIN=auto CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
    CC="zig cc -target aarch64-macos.11.1 -isysroot /sdk -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks" \
    CXX="zig c++ -target aarch64-macos.11.1 -isysroot /sdk -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks" \
    CGO_LDFLAGS="--sysroot /sdk -F/sdk/System/Library/Frameworks -L/usr/lib" \
    GOFLAGS="-buildmode=pie -ldflags=-w"
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache \
    fyne package -os darwin && mkdir /out && mv ./*.app /out/

FROM fyneio/fyne-cross-images:linux AS linux-amd64
WORKDIR /app
COPY . .
COPY --from=fyne-cli /usr/local/bin/fyne /usr/local/bin/fyne
ENV GOTOOLCHAIN=auto CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    CC="zig cc -target x86_64-linux-gnu.2.38 -isystem /usr/include -L/usr/lib/x86_64-linux-gnu" \
    CXX="zig c++ -target x86_64-linux-gnu.2.38 -isystem /usr/include -L/usr/lib/x86_64-linux-gnu"
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache \
    fyne package -os linux -release && mkdir /out && mv ./*.tar.xz /out/

FROM fyneio/fyne-cross-images:windows AS windows-amd64
WORKDIR /app
COPY . .
COPY --from=fyne-cli /usr/local/bin/fyne /usr/local/bin/fyne
# Windows uses package mode because fyne's release mode for windows only
# runs on windows hosts.
ENV GOTOOLCHAIN=auto CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
    CC="zig cc -target x86_64-windows-gnu -Wdeprecated-non-prototype -Wl,--subsystem,windows" \
    CXX="zig c++ -target x86_64-windows-gnu -Wdeprecated-non-prototype -Wl,--subsystem,windows"
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache \
    fyne package -os windows && mkdir /out && mv ./*.exe /out/

FROM scratch
COPY --from=darwin-amd64 /out /darwin-amd64
COPY --from=darwin-arm64 /out /darwin-arm64
COPY --from=linux-amd64 /out /linux-amd64
COPY --from=windows-amd64 /out /windows-amd64
