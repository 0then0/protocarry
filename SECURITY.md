# Security policy

## Supported versions

Security fixes target the latest published release, currently v0.1.1. Older releases are not maintained separately. Upgrade before checking whether a problem persists.

## Reporting a vulnerability

Use GitHub's [private vulnerability reporting](https://github.com/0then0/protocarry/security/advisories/new) when it is available. If that page does not offer reporting, open an issue titled “Request for private security contact” with no vulnerability details, payloads or affected data. The maintainer, [@0then0](https://github.com/0then0), can arrange a private channel. Do not disclose an exploit in a public issue or pull request before coordination.

Include the affected version or commit, operating system and architecture, reproduction steps with synthetic data, expected and observed behavior, and the practical impact. Describe whether the issue requires a malicious adapter, descriptor, message or evidence bundle. Remove credentials and private application data from all attachments.

This is a volunteer-maintained project with no guaranteed response or remediation deadline. Coordinate disclosure with the maintainer. There is no bug bounty program.

## Trust boundaries

ProtoCarry executes the configured adapter with the user's permissions. Adapters can access files, the network and other resources available to that user. Timeouts, process-group cleanup and resource limits are not a sandbox. Run only trusted adapters, or provide isolation outside ProtoCarry.

Evidence contains binary messages, descriptors, command arguments, paths and stderr. Treat it as potentially sensitive. Replay verifies stored artifact hashes and assertions, but hashes do not authenticate the author of a bundle or make its adapter safe. Inspect configuration and use explicit adapter and working-directory overrides when replaying evidence from another source.

A semantic FAIL is a violated preservation contract, not automatically a vulnerability. See the [contract reference](docs/contract.md) for supported inputs, limits and outcome definitions.
