# Security policy

## Supported versions

Security fixes target the latest published release, currently v0.1.3. Older releases are not maintained separately. Upgrade before checking whether a problem persists.

## Reporting a vulnerability

Use GitHub's [private vulnerability reporting](https://github.com/0then0/protocarry/security/advisories/new) when it is available. If that page does not offer reporting, open an issue titled “Request for private security contact” with no vulnerability details, payloads or affected data. The maintainer, [@0then0](https://github.com/0then0), can arrange a private channel. Do not disclose an exploit in a public issue or pull request before coordination.

Include the affected version or commit, operating system and architecture, reproduction steps with synthetic data, expected and observed behavior, and the practical impact. Describe whether the issue requires a malicious adapter, descriptor, message or evidence bundle. Remove credentials and private application data from all attachments.

This is a volunteer-maintained project with no guaranteed response or remediation deadline. Coordinate disclosure with the maintainer. There is no bug bounty program.

## Trust boundaries

### Historical runtime validation dependencies

The npm aliases in `validation/protobufjs` pin unmodified historical protobuf.js releases as compatibility test subjects. They are not part of the Go CLI or its release archives. Do not use these packages as a dependency recommendation for an application.

The following advisories were reviewed for the current validation relay:

- [GHSA-j3f2-48v5-ccww](https://github.com/advisories/GHSA-j3f2-48v5-ccww): an unterminated option in untrusted `.proto` text can hang parsing. The relay loads only the fixed, checked-in `validation/protobufjs/old.proto`; callers cannot select a schema path or provide schema text through stdin. stdin is decoded as binary Protobuf.
- [GHSA-jfj6-75fj-8934](https://github.com/advisories/GHSA-jfj6-75fj-8934): string-map parsing in the optional Text Format extension can mutate the returned map object's prototype. The relay does not import that extension or parse Text Format input, and its fixed schema has no maps.

These vulnerable input paths are not exercised by the current suite. Retain the historical versions and measurements rather than replacing the test subjects to silence dependency alerts. Reassess this decision if the relay starts accepting external schemas, loading Text Format extensions or exposing parsing to untrusted input. Run the suite only in a trusted checkout.

### Adapter execution and evidence

ProtoCarry executes the configured adapter with the user's permissions. Adapters can access files, the network and other resources available to that user. Timeouts, process-group cleanup and resource limits are not a sandbox. Run only trusted adapters, or provide isolation outside ProtoCarry.

Evidence contains binary messages, descriptors, command arguments, paths and stderr. Treat it as potentially sensitive. Replay verifies stored artifact hashes and assertions, but hashes do not authenticate the author of a bundle or make its adapter safe. Inspect configuration and use explicit adapter and working-directory overrides when replaying evidence from another source.

A semantic FAIL is a violated preservation contract, not automatically a vulnerability. See the [contract reference](docs/contract.md) for supported inputs, limits and outcome definitions.
