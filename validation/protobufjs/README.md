# protobuf.js runtime validation

This validation suite checks whether published protobuf.js packages preserve fields unknown to an application's older schema during binary decode/encode. ProtoCarry independently decodes the result with the newer schema and checks the selected fields.

The npm lockfile pins unmodified protobuf.js 8.6.1, 8.6.2 and 8.8.0 through package aliases, together with their dependency integrity hashes. Node.js is needed for this suite, not for the ProtoCarry CLI.

## Run the suite

From the repository root, with Go 1.27.x, Node.js 24.x and npm:

```sh
make validation
```

This builds the CLI and Go adapter, installs the locked npm packages, tests the runner and executes the measurements. The individual steps are:

```sh
make build
cd validation/protobufjs
npm ci --ignore-scripts --no-audit --no-fund
npm test
npm run validate
```

Each run requires a new `runs/` directory. Move any previous run aside before repeating the command. Ordinary validation writes only `runs/`; it leaves the checked-in measurements and evidence unchanged.

The runner also accepts `PROTOCARRY_BIN` and `PROTOCARRY_GO_RELAY` environment variables to test alternate CLI and Go adapter binaries.

## Fixtures and assertions

The [relay](relay.cjs) loads only the older schema, decodes stdin and encodes the resulting object. It logs the installed package version, Node.js version and reader option on stderr. The newer schema and preservation assertions are available only to ProtoCarry's oracle.

- **Simple:** `validation.Envelope { id: "A", future_note: "future" }`. The older schema knows only `id = 1`. ProtoCarry selects `future_note (#2)` and controls `id (#1)`.
- **Nested:** `validation.NestedEnvelope { id: "A", child: { label: "seed", future_note: "future" } }`. Both schemas know `child = 3` and `child.label = 1`; the newer child adds `future_note = 2`. The contract selects `child.future_note (#3/#2)` and controls `id` and `child.label`.

For each runtime, option and shape, the suite performs three checks of the seed and a generated distinct-value case, plus three exact-input baseline replays. It verifies input hashes, replay outcomes, observed protobuf.js metadata and report stability. An official Go runtime v1.36.12 binary adapter provides a preserving control.

Descriptors and seeds are included. Their companion [old](old.proto) and [new](new.proto) schema files describe the fixtures. Regenerate the binaries with `make fixtures` from the repository root.

## Recorded results

Both simple and nested fixtures produce:

- protobuf.js **8.6.1 default**: PASS.
- protobuf.js **8.6.2 and 8.8.0 default**: FAIL; the added string becomes `""`.
- protobuf.js **8.6.2 and 8.8.0 preservation opt-in**, `reader.discardUnknown = false`: PASS.
- **Go v1.36.12 preserving adapter**: PASS.

The [v0.1.1 measurement](results-v0.1.1.json) was recorded on 2026-10-05 on macOS arm64 with Go 1.27.1 and Node.js 24.21.0. It includes 108 executions in the original matrix, 27 in the empty-message controls below and eight legacy replays: 143 executed cases with no skips. Repeated reports and outcomes were stable. The recorded platform identifies the measurement environment; CI runs the suite on both supported arm64 platforms.

Measurements contain package and descriptor hashes, runtime versions, expected/actual assertions and per-trial outcomes. Representative current cases are in [evidence-v0.1.1/](evidence-v0.1.1/). The original [results.json](results.json) and [evidence/](evidence/) were produced by v0.1.0 and are kept unchanged as backward-compatibility fixtures. The runner replays all eight legacy cases with portable adapter overrides and checks their inputs, assertions, outcomes and reject policy.

Full results from a new run are in `runs/summary.json` and the individual case directories. CI compares runtime outcomes with the recorded measurements and uploads the run evidence for inspection.

## Replay a legacy case

After `make build` and installing the locked packages, run from the repository root:

```sh
mkdir -p evidence
./bin/protocarry replay \
  -case validation/protobufjs/evidence/8.6.2-default-simple/s001-baseline \
  -out evidence/legacy-discard \
  -working-dir "$PWD" \
  -adapter '["node","validation/protobufjs/relay.cjs","pb862","default","validation.Envelope"]'
```

The default-discard case exits `1` with FAIL. Replacing `default` with `preserve` and choosing a new output directory gives PASS on the same saved input and contract. Explicit overrides make the bundle usable in a different checkout. Replacing the adapter clears its old declared runtime label; stderr records the observed protobuf.js runtime.

## Empty-message controls

These controls select only `future_note (#2)` and explicitly use `empty_output: message`:

- protobuf.js 8.6.2 default, input `validation.Envelope { future_note: "future" }`: the older schema's decode/encode returns zero bytes. ProtoCarry reports FAIL with expected `"future"`, actual `""`.
- protobuf.js 8.6.2 preservation opt-in, the same input and contract: PASS with expected and actual `"future"`.
- Go v1.36.12 preserving adapter, a zero-byte seed: baseline PASS (`""` remains `""`), and the generated nondefault case also passes.

Each control has three checks and three exact-input baseline replays. Run the negative example from the repository root:

```sh
./bin/protocarry replay \
  -case validation/protobufjs/evidence-v0.1.1/8.6.2-default-only-new/s001-baseline \
  -out evidence/empty-discard \
  -working-dir "$PWD" \
  -adapter '["node","validation/protobufjs/relay.cjs","pb862","default","validation.Envelope"]'
```

It exits `1`. Replay the same case with preservation enabled:

```sh
./bin/protocarry replay \
  -case validation/protobufjs/evidence-v0.1.1/8.6.2-default-only-new/s001-baseline \
  -out evidence/empty-preserve \
  -working-dir "$PWD" \
  -adapter '["node","validation/protobufjs/relay.cjs","pb862","preserve","validation.Envelope"]'
```

This exits `0`. Both commands use the saved output policy; replay has no policy override. The empty-seed Go control also exits `0`:

```sh
./bin/protocarry replay \
  -case validation/protobufjs/evidence-v0.1.1/v1.36.12-preserve-empty-seed/s001-baseline \
  -out evidence/empty-seed \
  -working-dir "$PWD" \
  -adapter '["./bin/demo-adapter","-schema","validation/protobufjs/old.pb","-message","validation.Envelope","-mode","preserve"]'
```

All output directories must be new. Create the parent `evidence/` directory first if it does not exist.

## Record measurements

Use ordinary `npm run validate` to check behavior without replacing recorded data. To record the current release's measurement set, preserve any existing `runs/` and `evidence-v0.1.1/` directories elsewhere, then run from this directory:

```sh
node validate.cjs --record
```

Recording always writes `results-v0.1.1.json` and representative cases under `evidence-v0.1.1/`. It never replaces `results.json` or the legacy `evidence/`. An existing evidence destination is rejected before measurement. Measurement or evidence-copy failure leaves the results file unchanged. The former `--record-v011` spelling is accepted as an alias.

Inspect assertions, runtime metadata and hashes before updating recorded measurements. If outcomes differ, identify the cause before changing the baseline.

## Interpret a failure

[Upstream PR #2209](https://github.com/protobufjs/protobuf.js/pull/2209) introduced binary unknown-field preservation. [PR #2310](https://github.com/protobufjs/protobuf.js/pull/2310) made discard the default while retaining preservation opt-in. The [8.6.2 README](https://github.com/protobufjs/protobuf.js/blob/protobufjs-v8.6.2/README.md) documents that policy.

Default discard is valid runtime behavior. It violates the preservation contract used by this suite when a selected added field is lost. A FAIL identifies the loss at the application's output boundary; it does not identify the internal step responsible or imply universal behavior across other fixtures and runtimes.
