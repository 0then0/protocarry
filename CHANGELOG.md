# Changelog

User-visible changes are recorded here. Published versions also have detailed release notes and downloadable archives on [GitHub Releases](https://github.com/0then0/protocarry/releases).

## Unreleased

## 0.1.3

- Native macOS x64 (Intel) and Linux x64 binary archives, built and tested on native runners alongside the existing arm64 archives. Windows remains unsupported. The CLI, evidence format and schema scope are unchanged; replay also accepts format `2` evidence produced by `0.1.3`.

See the [v0.1.3 release notes](docs/releases/v0.1.3.md).

## 0.1.2

### Added

- Contributor, conduct and security policies, issue and pull request templates, and shared editor settings.
- A Kotlin/JVM external library compatibility case for kotlinx.serialization 1.12.0 unknown-field preservation, with a separate CI job. It needs a JDK only under `validation/kotlin-serialization`; the CLI is unchanged.

### Changed

- Explicit LF text handling while preserving binary descriptor and evidence bytes.
- CI cancels superseded runs for the same branch or pull request; CI and release jobs have time limits.
- Replay accepts format `2` evidence produced by `0.1.1` and `0.1.2`. Evidence format and CLI behavior are otherwise unchanged.
- Refreshed project icon.

See the [v0.1.2 release notes](docs/releases/v0.1.2.md).

## 0.1.1

- Opt-in `empty_output: "message"` treats successful zero-byte adapter output as a valid message and checks its values and presence. The default remains `reject`.
- Evidence format 2 records the output policy and immediate replay source producer.
- Replay accepts authentic v0.1.0 format 1 evidence with its legacy policy and validates supported producer/format combinations before adapter execution.
- Validation adds empty-message controls and replay of all eight legacy cases.

See the [v0.1.1 release notes](docs/releases/v0.1.1.md).

## 0.1.0

- Initial CLI with `check`, saved-input `replay`, bounded deterministic cases and decoded value/presence assertions.
- stdin/stdout adapters, per-case evidence, Go examples and pinned protobuf.js compatibility measurements.
- Native macOS arm64 and Linux arm64 archives with SHA-256 checksums.

See the [v0.1.0 release notes](docs/releases/v0.1.0.md).
