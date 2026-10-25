# Contributing to ProtoCarry

Bug reports, documentation improvements and focused pull requests are welcome. Participation follows the [Code of Conduct](CODE_OF_CONDUCT.md). Report vulnerabilities through the [security policy](SECURITY.md).

## Before making a change

Search existing [issues](https://github.com/0then0/protocarry/issues) and pull requests first. For a new public option, schema feature, dependency or substantial design change, open an issue describing the use case and proposed behavior before implementing it. Small fixes can go directly to a pull request.

For a bug, include the CLI version, operating system and architecture, adapter/runtime versions, expected and actual behavior, exit code and a minimal reproduction. Use synthetic descriptors and messages. Evidence can contain application data, paths, command arguments and stderr; inspect and redact it before sharing.

## Development

Use Go 1.27.x, Make and macOS or Linux. Node.js 24.x and npm are needed only for external runtime validation. See the [development guide](docs/development.md) for complete commands and platform scope.

```sh
go mod download
make build
make test vet
```

Format Go changes with `gofmt`. Follow [.editorconfig](.editorconfig), keep changes focused and add regression tests for behavior changes. Use existing dependencies where possible. Do not commit generated build output, secrets or local evidence.

For fixture changes, run `make fixtures` and review the binary changes. For adapter, oracle, replay or runtime-validation changes, also run `make demo validation` with fresh output directories. The development guide explains how to handle previous runs. Documentation links are checked by the Go tests.

The committed v0.1.0 evidence and original runtime measurements are compatibility fixtures. Keep them unchanged. Investigate changed outcomes before updating current expectations or recording new evidence; document the runtime versions and reason. The npm package aliases are intentional historical versions, not dependencies to upgrade automatically.

## Pull requests

Describe the problem, resulting behavior and checks actually run, including any checks you could not run. Keep unrelated changes separate. Update public documentation and the Unreleased section of [CHANGELOG.md](CHANGELOG.md) when behavior or contributor workflows change.

Use English Conventional Commit subjects such as `fix: preserve optional presence` or `docs: clarify replay setup`. CI must pass on both supported platforms before merging. Maintainers review changes; opening a pull request does not grant merge or release permissions.

## Releases

Maintainers prepare releases using the [release procedure](docs/development.md#prepare-binary-archives). A version tag publishes native archives after verification. Do not create a release tag as part of an ordinary contribution.
