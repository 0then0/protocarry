<p align="center"><img src="assets/icon.svg" width="112" height="112" alt="Newer Protobuf fields carried through an older application" /></p>

# ProtoCarry

[![CI](https://img.shields.io/github/actions/workflow/status/0then0/protocarry/ci.yml?branch=main&label=CI)](https://github.com/0then0/protocarry/actions/workflows/ci.yml)
[![MIT license](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8.svg)](go.mod)
[![Version](https://img.shields.io/badge/version-0.1.0-0f766e.svg)](docs/contract.md)

A producer writes a message with schema v2, an application reads and writes it with schema v1, and a consumer reads it with v2. Every step can succeed while a mapper, a fresh object or a JSON conversion loses fields that v1 does not know.

ProtoCarry runs your application adapter on valid binary fixtures, decodes its output independently with the newer schema, and checks the fields you explicitly require. It saves the input, output, descriptors and assertions so a failure can become a regression test.

Use it for local correctness testing with your own or prepared fixtures. A finding identifies a loss at the application's output boundary; it does not identify the internal step that removed the field.

## Quick start

Build from source with Go 1.27.x and Make. The CLI is a standalone binary; it does not require Node.js or protoc at runtime.

```sh
git clone https://github.com/0then0/protocarry.git
cd protocarry
go mod download
make build
./bin/protocarry version
mkdir -p evidence
./bin/protocarry check -config examples/demo/preserve.json -out evidence/preserve
```

The preserving adapter uses the official Go runtime to decode and encode with the older schema. All seven demo cases should pass. The descriptors and seed are included, so no schema compiler is needed for the demo.

Try a mapper that copies only known fields into a fresh object:

```sh
./bin/protocarry check -config examples/demo/copy.json -out evidence/copy
```

This command intentionally exits `1` and reports losses such as:

```text
FAIL s001-baseline: preservation contract violated
  field routing_hint (#2)
    expected: "future-route" (string)
    actual:   "" (string)
    older schema: field/path absent
```

Replay the saved input with the same contract:

```sh
./bin/protocarry replay -case evidence/copy/s001-baseline -out evidence/replay
```

The replay should also exit `1`. Output directories must be new; choose another name or move previous evidence aside before repeating a command.

## Test your application

You need two descriptor sets with imports, the fully qualified message name, at least one valid newer-schema binary seed, and an adapter executable. Create descriptors outside ProtoCarry:

```sh
protoc -I schemas/v1 --include_imports --descriptor_set_out=old.pb schemas/v1/example.proto
protoc -I schemas/v2 --include_imports --descriptor_set_out=new.pb schemas/v2/example.proto
```

Alternatively, use `buf build schemas/v1 -o old.pb` and `buf build schemas/v2 -o new.pb`.

The adapter reads one unframed binary message from stdin, calls your application's real read/write path, and writes the binary result to stdout. Put logs on stderr. Each case starts a fresh process. The result must represent the same logical root message.

A wrapper can call a serializer, protobuf-to-domain mapper, or local load/save workflow. It can be written in any language and does not need framework integration. See the runnable [Go demo adapter](cmd/demo-adapter/main.go) and [protobuf.js relay](validation/protobufjs/relay.cjs) for the process boundary. Replace their decode/encode operation with your application's path.

Save a configuration beside your descriptors and seed:

```json
{
  "old_descriptor": "old.pb",
  "new_descriptor": "new.pb",
  "message": "example.Envelope",
  "seeds": ["seed.bin"],
  "adapter": ["./my-application-adapter", "--workflow", "save-load"],
  "fields": ["routing_hint", "child.future_note"],
  "controls": ["id"],
  "limits": { "timeout_ms": 2000, "max_cases": 32 }
}
```

```sh
./bin/protocarry check -config carry.json -out evidence/application
```

`fields` selects additions whose values and explicit presence must survive. `controls` selects fields already known to the older schema. Known fields outside this contract may change intentionally.

Descriptor, seed and working-directory paths are relative to the config file. The adapter runs in that directory unless `working_dir` is set. Relative executable paths containing a slash resolve against the working directory; bare executable names use PATH. Other arguments are passed unchanged. ProtoCarry does not invoke a shell.

An adapter can refuse a fixture by exiting `75`, optionally explaining why on stderr. ProtoCarry reports `UNRESOLVED`. Other nonzero exits are infrastructure errors, not evidence of data loss.

## Cases and supported fields

ProtoCarry first runs each unchanged seed. It then creates a bounded, deterministic set of cases, varying one selected added field at a time. Every executed case checks all selected fields and controls against that case's input. Optional scalars receive an additional case with an explicitly present default value.

Version 0.1 supports proto3 additions of:

- Bool, all signed and unsigned integer scalar types, float and double.
- String and bytes.
- Optional scalars, including explicit zero, false, empty string and empty bytes.
- Fields inside existing singular messages, and added singular message fields with bounded depth.

Integers compare exactly. Floating-point comparison has no tolerance: signed zeros compare equal, all NaNs compare equal, and infinities retain their sign. NaN payload bits are outside the contract. Implicit scalar defaults do not acquire an invented presence requirement.

Missing parent messages prevent case generation by default. Set `allow_create_parents: true` to permit the necessary creation. Skipped cases are reported as unresolved.

There is no repeated/map traversal, enum-semantics testing, real-oneof support, extensions, proto2 or Editions support. Unchanged collections may exist outside the contract. Unsupported changes in the reachable schema stop adapter execution, including changes to an existing field's name, number, type or presence. See the [contract reference](docs/contract.md) for the full scope and limits.

## Read the result

- **PASS**, exit `0`: all planned cases executed and all required assertions held. It applies only to the tested inputs, contract, adapter path and environment.
- **FAIL**, exit `1`: a supported, successful adapter execution violated an assertion. Inspect the field's expected and actual values or presence.
- **UNRESOLVED**, exit `3`: an unsupported model, skipped case or controlled refusal prevented complete verification.
- **INFRASTRUCTURE_ERROR**, exit `4`: a launch, timeout, transport, resource or persistence failure prevented verification.
- Configuration errors exit `2` before cases launch. Invalid seeds are configuration errors.

The aggregate priority is infrastructure error, failure, unresolved, then pass. A failure can coexist with unverified cases; inspect `incomplete`, counts and individual outcomes in `report.json`. No skipped case counts as a pass.

Evidence is saved before the final CLI result. Each case includes available binary input/output, both descriptor sets, stderr and a manifest with argv, limits, resolved field names/numbers, assertions, process status, artifact hashes and versions. Comparison uses decoded semantics, not binary equality.

Replay verifies saved hashes and expectations and sends the exact saved input. To test a fixed adapter or relocate a bundle, supply explicit replacements:

```sh
./bin/protocarry replay -case CASE_DIR -out NEW_DIR \
  -working-dir "$PWD" -adapter '["./bin/my-fixed-adapter"]'
```

Require exit `0` from this command in your regression test. The input and contract remain unchanged. Application resources are not archived; the selected executable and its dependencies must be available. See [evidence and replay](docs/contract.md#evidence-and-replay) for details.

## Demo paths

`make demo` runs four paths with the same schemas, seed and contract:

- `preserve`: binary decode/encode with unknown-field preservation, PASS.
- `copy`: copy known fields into fresh objects, FAIL.
- `json`: an old-schema ProtoJSON roundtrip, FAIL.
- `mutate`: change the unasserted known `score` field while preserving the contract, PASS.

These results illustrate how a contract handles application choices. Intentional removal of unknown fields is acceptable when the contract does not require them.

## External runtime compatibility case

The included [protobuf.js case](validation/protobufjs/README.md) measures published, locked upstream packages with the same contract for simple and nested messages:

- 8.6.1 default: PASS.
- 8.6.2 and 8.8.0 default: FAIL.
- 8.6.2 and 8.8.0 with `reader.discardUnknown = false`: PASS.
- Official Go runtime v1.36.12 binary relay: PASS.

The recorded run contains three checks and three exact-input replays per combination, with stable reports and outcomes. [Measurements](validation/protobufjs/results.json) and [replayable bundles](validation/protobufjs/evidence/) are included.

These results show a dependency-upgrade behavior change affecting this contract. Default discard is documented protobuf.js behavior, not evidence of an upstream bug. This is a runtime compatibility case, not an adoption claim or a newly discovered vulnerability.

## How it fits

[Buf breaking](https://buf.build/docs/breaking/) checks schemas against compatibility rules. Compatible additions do not tell you whether an application's mapper or load/save workflow retains them. ProtoCarry tests that path.

A [runtime conformance suite](https://github.com/protocolbuffers/protobuf/tree/main/conformance) tests broad protocol implementation behavior. ProtoCarry checks a small, application-specific preservation obligation. It does not provide universal forward compatibility, fuzzing, a schema registry or a proxy.

## Platforms and development

Tested CLI and release targets are macOS arm64 and Linux arm64. Process groups provide bounded timeout and pipe cleanup; they do not isolate arbitrary processes or descendants that escape the group. Empty stdout is a transport error, even if an empty Protobuf message would otherwise be valid.

For tests, fixture generation, CI and archive preparation, see the [development guide](docs/development.md). Node.js is required only for the external validation tools. ProtoCarry is licensed under [MIT](LICENSE).
