# Photos Export for Proton Drive

A small desktop app that downloads every photo from the Photos section
of your Proton Drive to a local folder.

Sign in with your Proton account (TOTP two-factor and two-password mode
are supported), pick a folder, and press download. Files are named
`YYYYMMDD_HHMMSS_originalname` after their capture time, and downloads
can be paused and resumed. Already-downloaded files are skipped, so you
can stop the app and pick up where you left off later.

## Install

Download the build for your platform from the
[releases page](https://github.com/tom-barone/proton-drive-photos-export/releases):

- **macOS**: unzip and move the app to Applications. The app is not
  signed, so the first launch must be started with right-click > Open,
  then Open in the warning dialog.
- **Windows**: download and run the .exe.
- **Linux**: unpack the tar.xz and run `make install`, or just run the
  binary inside.

## Notes

- Your credentials go only to the Proton API. The session token is
  cached in your user config directory (`ProtonPhotosExport/session.json`)
  so you don't have to sign in every time.
- Logs are written to `ProtonPhotosExport/app.log` in your user cache
  directory (`~/Library/Caches` on macOS, `%LocalAppData%` on Windows,
  `~/.cache` on Linux).

## Building from source

Requires [Go](https://go.dev). The fyne CLI is pinned as a tool in
go.mod, so no extra installs are needed (`just package-all` also needs
Docker running for the cross-builds; on a non-Mac host the darwin leg
additionally needs a macOS SDK image, see `fyne-cross darwin-sdk-extract`
and note Apple only licenses its SDK for use on Apple hardware):

```
just run          # run from source
just build        # plain binary for the current platform
just package      # installable bundle for the current platform
just package-all  # bundles for darwin, linux and windows
just test         # run tests
```

Releases are built by GitHub Actions when a `v*` tag is pushed.

## License

GPL-3.0, see [LICENSE](LICENSE).
