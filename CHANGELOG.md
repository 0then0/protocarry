# Changelog

User-visible changes are recorded here. Published versions also have detailed release notes and downloadable archives on [GitHub Releases](https://github.com/0then0/protocarry/releases).

## Unreleased

### Added

- Contributor, conduct and security policies, issue and pull request templates, and shared editor settings.

### Changed

- Explicit LF text handling while preserving binary descriptor and evidence bytes.
- CI cancels superseded runs for the same branch or pull request; CI and release jobs have time limits.

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
