# protobuf.js runtime compatibility case

This example measures whether published protobuf.js versions preserve fields they do not know during a binary decode/encode roundtrip. It uses ProtoCarry's newer-schema oracle with the same preservation contract for every version and option.

npm aliases load unmodified upstream packages 8.6.1, 8.6.2 and 8.8.0 side by side. The lockfile pins package tarballs and the `long` dependency with integrity hashes. The CLI itself has no JavaScript dependency.

## Run the measurements

From the repository root, with Go 1.27.x, Node.js 24.x and npm:

```sh
make validation
```

Or run the steps individually:

```sh
make build
cd validation/protobufjs
npm ci --ignore-scripts --no-audit --no-fund
npm test
npm run validate
```

The runner requires a new `runs/` directory. Move previous measurements aside before running again. Normal validation writes only `runs/`; it does not update the committed baseline or representative evidence.

The included descriptors and seeds require no compiler. Regenerate them with `make fixtures` from the repository root. Their companion `.proto` files describe the same schemas.

## Adapter and contract

[relay.cjs](relay.cjs) reads a binary message, decodes it using the old schema and encodes the resulting object with the upstream runtime. It logs the installed package version, Node.js version and reader option on stderr. It neither loads the newer schema nor decides whether preservation succeeded.

The two fixtures are:

- **Simple:** `validation.Envelope { id: "A", future_note: "future" }`. The older schema has only `id = 1`. The contract selects `future_note (#2)` and controls `id (#1)`.
- **Nested:** `validation.NestedEnvelope { id: "A", child: { label: "seed", future_note: "future" } }`. Both schemas know singular `child = 3` and its `label = 1`; the newer child adds `future_note = 2`. The contract selects `child.future_note (#3/#2)` and controls `id` and `child.label`. No parent creation is needed.

For each version/option/shape, ProtoCarry tests the exact seed and one generated distinct-value case. The runner performs three checks and three saved-input baseline replays, verifies input hashes and replay outcomes, and checks report stability. It records measured PASS/FAIL outcomes without version-specific oracle expectations.

A positive control uses the official Go runtime v1.36.12 binary adapter with the same descriptors, seeds and contracts. ProtoCarry's oracle also uses the official Go runtime, independently of each adapter process.

## Recorded results

The [recorded measurement](results.json) was made on 2026-10-05 on macOS arm64 with Go 1.27.1 and Node.js 24.21.0. Both simple and nested fixtures produced:

- protobuf.js **8.6.1 default**: PASS.
- protobuf.js **8.6.2 default**: FAIL; the selected added string becomes `""`.
- protobuf.js **8.6.2 with `reader.discardUnknown = false`**: PASS.
- protobuf.js **8.8.0 default**: FAIL; the selected added string becomes `""`.
- protobuf.js **8.8.0 with `reader.discardUnknown = false`**: PASS.
- **Go v1.36.12 binary relay**: PASS.

The twelve combinations contain 36 check runs, 72 check case executions and 36 replay executions: 108 executed cases with no skips. Repeated reports and replay outcomes were stable.

`results.json` contains per-trial hashes and expected/actual assertions. Representative positive and negative cases are in [evidence/](evidence/). Full new measurements appear in `runs/summary.json` and individual case directories; `runs/` is ignored by Git.

CI compares measured outcomes with the committed baseline as a separate regression check. If they differ, inspect the evidence before changing the baseline.

## Replay a saved case

From the repository root after `make build` and dependency installation:

```sh
mkdir -p evidence
./bin/protocarry replay \
  -case validation/protobufjs/evidence/8.6.2-default-simple/s001-baseline \
  -out evidence/relocated-replay \
  -working-dir "$PWD" \
  -adapter '["node","validation/protobufjs/relay.cjs","pb862","default","validation.Envelope"]'
```

The recorded default-discard case should FAIL with exit `1`. Replacing `default` with `preserve` in the argv gives the positive control, expected PASS. The saved input and contract remain unchanged. Use a new output directory for each replay.

Overrides make the bundle usable in another checkout despite its original application paths. Replacing the adapter clears the saved runtime declaration; the relay logs the observed installed version on stderr.

## Update the recorded baseline

Use this only when you intend to replace the committed measurement. Inspect the new assertions and package versions first. Keep the old results and evidence while preparing the replacement.

From `validation/protobufjs`, move any existing `runs/` aside. Move `evidence/` to a backup location outside the repository, then run:

```sh
node validate.cjs --record
```

The command measures a fresh matrix, creates representative evidence, and then updates `results.json`. It refuses before measurement if the evidence destination already exists, leaving results and evidence unchanged. It also refuses to reuse `runs/`. If measurement or evidence copying fails, `results.json` is not updated; inspect any partial new directories before retrying.

Review the new `results.json` and evidence together before committing them. `npm test` checks that refusal preserves existing artifacts and does not create a measurement directory.

## Interpret a failure

[Upstream PR #2209](https://github.com/protobufjs/protobuf.js/pull/2209) introduced binary unknown-field preservation. [PR #2310](https://github.com/protobufjs/protobuf.js/pull/2310) deliberately made discard the default while retaining preservation opt-in. The [8.6.2 README](https://github.com/protobufjs/protobuf.js/blob/protobufjs-v8.6.2/README.md) documents that policy.

Default discard is valid runtime behavior. It becomes a ProtoCarry FAIL when it violates the chosen contract. The version comparison demonstrates a dependency-upgrade behavior change for these fixtures; it is not evidence of an upstream bug, a newly discovered vulnerability or external adoption. A finding does not identify the internal application step responsible for the loss.
