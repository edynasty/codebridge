# Binary releases

GitHub Releases are produced only when a version tag matching `v*` is pushed.

The release workflow first runs formatting checks, unit tests, and `go vet`, then builds static binaries with `CGO_ENABLED=0` and embeds the tag as the binary version.

## Release assets

Each release contains `codebridge-client`, `codebridge-manager`, and `codebridge-doctor`.

| Platform | Asset |
| --- | --- |
| macOS Apple Silicon | `codebridge_vX.Y.Z_darwin_arm64.tar.gz` |
| macOS Intel | `codebridge_vX.Y.Z_darwin_amd64.tar.gz` |
| Linux x86_64 | `codebridge_vX.Y.Z_linux_amd64.tar.gz` |
| Linux arm64 | `codebridge_vX.Y.Z_linux_arm64.tar.gz` |
| Windows x86_64 | `codebridge_vX.Y.Z_windows_amd64.zip` |

A `SHA256SUMS` file is published alongside the archives.

## Verify

On macOS/Linux:

```bash
sha256sum -c SHA256SUMS
```

On macOS, `shasum -a 256 <archive>` can also be used to compare a single asset manually.

## Install the local Client on macOS Apple Silicon

Extract the `darwin_arm64` archive:

```bash
tar -xzf codebridge_vX.Y.Z_darwin_arm64.tar.gz
cd codebridge_vX.Y.Z_darwin_arm64
./codebridge-client --version
```

Then place the binary somewhere on your PATH, for example:

```bash
sudo install -m 0755 codebridge-client /usr/local/bin/codebridge-client
```

On Apple Silicon systems using a Homebrew-style user-writable prefix, another common location is `/opt/homebrew/bin`.

## Install on Linux

```bash
tar -xzf codebridge_vX.Y.Z_linux_amd64.tar.gz
sudo install -m 0755 codebridge-client /usr/local/bin/codebridge-client
codebridge-client --version
```

Use the `linux_arm64` archive on ARM64 servers/workstations.

## Windows

Extract the Windows ZIP and place `codebridge-client.exe` in a directory included in `PATH`.

```powershell
.\codebridge-client.exe --version
```

## Build from source

A release is not required for development:

```bash
make test
make build VERSION=dev
```

The three binaries are written to `bin/`.

## Creating a release

After a commit has passed CI and a real deployment has been validated:

```bash
git tag -a v0.3.0 -m "CodeBridge v0.3.0"
git push origin v0.3.0
```

The GitHub Actions release workflow creates the archives, checksums, and GitHub Release automatically.

Do not create a stable release tag merely because the project compiles. For CodeBridge, a release should also have passed an actual Manager/Client connection and MCP/OAuth smoke test.
