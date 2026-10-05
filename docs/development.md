# Development and verification

This guide covers building ProtoCarry, running its checks and preparing binary archives. For application configuration, start with the [README](../README.md) and [contract reference](contract.md).

## Requirements

- Go 1.27.x and Make.
- macOS or Linux for adapter subprocess tests.
- Node.js 24.x and npm for the external protobuf.js validation.
- `tar` and either `sha256sum` or `shasum` for release packaging.

The tested release targets are macOS arm64 and Linux arm64. Windows and Intel release artifacts are not provided. The CLI has no dependency on Node.js, protoc or Buf at runtime.

## Build and test

From the repository root:

```sh
go mod download
make build
make test vet
```

`make build` creates `bin/protocarry` and the example `bin/demo-adapter`. `make test` runs Go tests with the race detector. Tests use temporary fixtures and real adapter subprocesses; they cover semantic preservation, descriptor scope, generation limits, transport failures, report stability and saved-input replay.

The release packaging test checks that an archive and checksum can be copied to another directory and verified there. It also checks that a corrupted archive is rejected. Compilation is stubbed in this packaging test; `make release` builds and executes the actual native binary.

## Fixtures and demos

The demo and external validation include descriptor sets and binary seeds. Regenerate them with:

```sh
make fixtures
```

The fixture generator builds descriptor sets using official Go descriptor types. The companion `.proto` files document the schemas. CI checks that regeneration leaves the checked-in fixtures unchanged.

```sh
make demo
```

The preserving and permitted-mutation modes must pass. The copy and ProtoJSON modes must exit `1` because they violate the demo contract. Make treats those expected failures as successful checks.

Demo evidence is written to `evidence/demo-*`. These directories must not already exist; move them aside before repeating the command.

## External validation

```sh
make validation
```

This builds the binaries, installs the locked npm dependencies with `npm ci`, runs the validation-runner regression tests, and measures the protobuf.js/Go cases through ProtoCarry. Node.js is used only in this validation directory.

The runner writes `validation/protobufjs/runs/` and refuses to reuse it. Move prior measurements aside before running again. Normal validation does not change the committed results or representative evidence. See the [validation guide](../validation/protobufjs/README.md) for measurement interpretation, replay and baseline updates.

## CI

The [CI workflow](../.github/workflows/ci.yml) runs on macOS and Linux arm64 runners. It checks Go tests and vet, reproducible fixture generation, demos and external validation. Separate assertions compare measured runtime outcomes with the v0.1.0 baseline and v0.1.1 empty-message measurements. Both native runners also exercise authentic legacy replay fixtures. Evidence is uploaded as a workflow artifact, including when a check fails.

When a runtime outcome changes, inspect its assertions, input/output and recorded dependency versions before updating the baseline. Update a recorded outcome only after identifying and documenting the reason for the change. Keep the legacy replay fixtures unchanged.

## Prepare binary archives

Run on the native target platform:

```sh
make release
```

The script runs Go tests and vet, builds a standalone binary, requires version 0.1.1, and writes an archive and SHA-256 checksum under `bin/`. Each archive contains `protocarry`, `README.md` and `LICENSE`. The repository contains the linked documentation, examples and validation data.

For example, verify a macOS arm64 archive after copying both files into your download directory:

```sh
shasum -a 256 -c protocarry_0.1.1_darwin_arm64.tar.gz.sha256
tar -xzf protocarry_0.1.1_darwin_arm64.tar.gz
./protocarry version
```

On Linux, use `sha256sum -c protocarry_0.1.1_linux_arm64.tar.gz.sha256`.

The [release workflow](../.github/workflows/release.yml) prepares artifacts for the same two platforms. A manual run only prepares artifacts. Both native jobs verify the checksum and execute the extracted binary before uploading. Pushing a version tag publishes a GitHub release after both native builds and checksum checks pass. The tag must match the binary version, and release notes must exist at `docs/releases/<tag>.md`. Only the publication job has repository write permission. The local script never publishes a release.

## Interpreting verification

Passing tests and a preserving demo cover the exercised supported semantics. A ProtoCarry PASS applies only to its saved contract and cases. Unchecked fields, application paths, fixtures and dependency versions are outside that result.

Adapter binaries, their environment and external resources are not captured as a complete application snapshot. Log observed runtime versions on stderr when that information matters to a regression. Process-group cleanup is not a sandbox; application file writes, network activity and escaped descendants remain the adapter author's responsibility.
