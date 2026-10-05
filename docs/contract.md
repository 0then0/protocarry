# Preservation contract reference

This reference describes ProtoCarry v0.1 configuration, comparison rules, limits and evidence. For a runnable introduction, see the [README](../README.md).

## Configuration

`protocarry check -config CONFIG.json -out NEW_DIR` accepts one JSON object. Unknown keys and trailing JSON documents are rejected. A configuration file can be at most 1 MiB.

Required keys:

- `old_descriptor`: older-schema `google.protobuf.FileDescriptorSet`, including imports.
- `new_descriptor`: newer-schema descriptor set, including imports.
- `message`: the same fully qualified root message name in both sets.
- `seeds`: 1..128 paths to valid binary messages of the newer schema.
- `adapter`: a nonempty array of executable name and arguments.
- `fields`: at least one path to an added field that must be preserved.

Optional keys:

- `controls`: paths to fields known to the older schema that must also be preserved.
- `working_dir`: adapter working directory; defaults to the configuration directory.
- `allow_create_parents`: defaults to `false`; permits creating missing singular ancestors in generated cases when `true`.
- `limits`: resource limits listed below. Omitted limit keys receive defaults; explicit zero is invalid.
- `validation_runtime`: an object with `name`, `version` and `options` strings describing the adapter runtime. This is a declared label, not an observed version check. An adapter can log observed versions on stderr.

There can be at most 128 distinct paths across `fields` and `controls`. Use exact protobuf field names separated by dots, such as `child.future_note`. JSON names, field-number selectors and collection indices are not accepted. ProtoCarry resolves each component with the newer descriptors and saves its field-number sequence in evidence.

Descriptor, seed and working-directory paths resolve relative to the config file. A relative executable path containing a slash resolves against the working directory. Bare executable names use PATH. Other argv entries are passed verbatim, and the adapter inherits the invoking environment. No shell is used.

## Input validation and schema scope

Before starting any adapter, ProtoCarry validates configuration, both descriptor sets, all selected paths and every seed. Descriptor sets must resolve all imports and are limited to 8 MiB each. The combined binary size of seeds is limited to 16 MiB. Invalid descriptors, missing files/imports, invalid paths or limits, and invalid or oversized seeds are configuration errors.

Supported evolution consists of proto3 field additions. Existing reachable fields must retain their names, numbers, types, cardinality, explicit presence, packing and JSON names. Referenced message names must remain the same.

Unsupported models produce `UNRESOLVED` before dependent cases run:

- Proto2, Editions, extensions, real oneofs and reachable enum fields, including unchanged enums.
- Added repeated fields, maps, enums or real-oneof members.
- Changes beneath repeated/map fields. Unchanged collections may exist outside the selected contract.
- Selected paths through collections, and selected messages containing repeated/map/enum/real-oneof descendants or recursive message structures.
- Paths or selected message descendants deeper than `max_depth`.

Unreachable declarations and services are outside this check. Custom options, reservations, application validation and business rules are not evaluated. Passing the scope check is not a general schema-compatibility certificate.

`fields` must select additions, including descendants of added singular messages. `controls` must select paths known to the older schema. Selecting an entire known message as a control asserts all of its newer-descriptor-known descendants, including additions; choose individual leaves if you want a narrower contract.

## What an assertion compares

Each case compares every selected field and control with that case's input. Known fields outside the contract may change, and unselected unknown fields may be intentionally discarded.

An assertion contains the selected value, explicit presence where the descriptor defines it, and the presence of each singular ancestor. Removing a parent is visible even if the selected descendant has a default value.

- Implicit scalars have no presence assertion. An absent scalar and an explicitly encoded default compare equal. Losing a nondefault value compares against the semantic default and fails.
- Optional scalars compare both value and presence. Explicit zero, false, empty string or empty bytes must stay present. Changing absence to presence also violates the contract.
- Integers compare exactly, including the full 64-bit range. Reports encode numeric values as decimal strings to avoid JSON number precision loss. Bytes are base64; strings use JSON escaping.
- Float and double compare decoded numerical values without tolerance. Signed zeros compare equal; all NaNs compare equal without distinguishing sign or payload; infinities retain their sign. Bit-level preservation is outside the contract.
- A selected message compares its presence and descriptor-known descendants recursively, ordered by field number. Unknown fields with no newer-schema descriptor are outside this semantic comparison.

The oracle uses the official Go runtime and `dynamicpb`. Binary decoding has a recursion limit of 64. Different valid field ordering or wire encoding does not cause failure. Artifact hashes identify files and are never the preservation criterion. Unknown length-delimited data is not guessed to be a message.

## Case generation and execution

All unchanged seed baselines are planned first. Added fields then follow seed order and configuration order. Each selected field receives a case with a small nondefault value; optional scalars also receive an explicitly present default-value case.

For a scalar case, only that selected field changes. A selected added message is a contract unit: it is created if absent, then one supported leaf is assigned a small value. Existing siblings remain unchanged. Empty messages are tested through presence. Generation clones the real seed, uses the newer descriptors, and verifies the generated binary by decoding it before execution.

A missing ancestor makes generation unresolved unless `allow_create_parents` is enabled. This option creates only the necessary singular ancestry. Baselines always use the original bytes. Generator errors and generated inputs exceeding their limit are unresolved, not application failures.

Cases run sequentially. A baseline failure does not prevent remaining field cases. `max_cases` counts adapter attempts, including baselines and launch failures. Cases beyond the budget remain in the report as unresolved. A skipped case without a generated input cannot be replayed.

## Resource limits

Values are bytes except for time, case count and depth:

- `timeout_ms`: default 2000; allowed 1..60000 per invocation.
- `max_cases`: default 64; allowed 1..1000 adapter attempts per run.
- `max_depth`: default 8; allowed 1..32 path components, including selected-message descendants.
- `max_message_bytes`: default 1048576; allowed 1..16777216 per input.
- `max_value_bytes`: default 64; allowed 1..4096 per descriptor-known string/bytes value in input or output, including unselected fields and unchanged collections.
- `max_stdout_bytes`: default 1048576; allowed 1..16777216 and cannot exceed `max_message_bytes`.
- `max_stderr_bytes`: default 65536; allowed 1..1048576.

If you lower `max_message_bytes` below the default stdout limit, lower `max_stdout_bytes` too. Increase `max_value_bytes` when real fixtures contain longer known strings or bytes. Generated values fit this limit; unknown wire data is bounded by total message size.

Planning is bounded by 128 seeds and 128 contract paths, with at most 32896 entries including optional variants. Evidence includes descriptors for skipped cases, so disk usage depends on the whole plan, not only executed cases. There is no separate disk quota. Each case manifest is limited to 8 MiB so successfully saved cases remain readable by replay. Exceeding that limit is a persistence error.

ProtoCarry does not impose adapter memory/CPU quotas or restrict its network access and file writes.

## Adapter protocol and cleanup

One process receives one unframed binary root message on stdin. It must return a nonempty, complete binary message of the same logical root on stdout. Logs belong on stderr. Protobuf binary has no root-type tag, so returning the intended type is an adapter requirement.

Exit `75` declares a controlled refusal. It may occur before reading all stdin; no assertion is evaluated. Other nonzero exits, empty or malformed stdout, incomplete input writes and resource-limit violations are infrastructure errors. An empty result is a transport error even when an empty root could be valid Protobuf.

Stdin, stdout and stderr are serviced concurrently. A successful pipe write is evidence of transport completion, not proof that the application consumed the bytes.

On macOS and Linux the adapter starts in a new process group. Timeout or output overflow kills that group with SIGKILL. Remaining group members are also killed after the parent exits. Output readers have up to 250 ms to drain; an incomplete drain is an infrastructure error. Cleanup can extend beyond the configured timeout.

Descendants that move to another session or group can escape this cleanup. ProtoCarry is a local test runner, not a process sandbox. Other operating systems receive an infrastructure outcome before adapter execution.

## Outcomes and exit codes

- `PASS` (`0`): all planned cases executed and all assertions were preserved. The result applies only to this contract, inputs, variants, adapter and environment.
- `FAIL` (`1`): a supported case completed successfully, but the independently decoded output violated an assertion.
- `UNRESOLVED` (`3`): an unsupported model, controlled refusal, skipped case or unavailable assertion prevented verification.
- `INFRASTRUCTURE_ERROR` (`4`): launch, timeout, transport, resource or evidence-writing failure.
- Configuration error (`2`): invalid configuration, descriptors or seeds, detected before adapter execution. No run report is created.

Aggregate priority is `INFRASTRUCTURE_ERROR > FAIL > UNRESOLVED > PASS`. A FAIL plus an unresolved case exits `1` with `incomplete: true`; any infrastructure case exits `4`, even when another case failed its contract. Individual outcomes and counts remain available.

`attempted_cases` counts adapter calls; `executed_cases` counts processes that started. An assertion has `preserved` only after the oracle evaluates it. Skipped and unsupported cases do not count as passes. A run with no cases cannot pass.

## Evidence and replay

The output directory must be new. Files use private permissions. The aggregate `report.json` lists the resolved contract, all planned cases, counts and outcomes. Each case directory contains:

- `input.bin` when an input is available.
- `output.bin` and `stderr.log` when the adapter started, retained up to their limits.
- `old.pb` and `new.pb` descriptor sets.
- `case.json` with effective argv, working directory, limits, versions, field names/numbers, expected/actual observations, process status and artifact SHA-256 hashes.

An output file can be empty, malformed or truncated; its case outcome explains the problem. A skipped generated case may have neither input nor output. Filesystem failures can leave a partial bundle without an aggregate report. The CLI prints its final result only after evidence has been saved successfully.

Reports omit timestamps and durations. Successful reports are stable within the same configuration, descriptors and environment; error text and partial output may vary. Saved manifests retain the application path context, including absolute executable/working-directory references where applicable. They do not archive the application or its resources.

```sh
protocarry replay -case CASE_DIR -out NEW_DIR
```

Replay checks artifact hashes, recomputes saved expectations from the input and descriptors, and executes only the exact saved input. It does not generate cases. Saved evidence must use the supported format and tool version; v0.1 accepts format `1` from tool `0.1.0`.

For relocation or a replacement adapter, use `-working-dir DIR` and `-adapter JSON_ARGV`. Replacing the adapter clears the old declared runtime label. The input, descriptors and preservation contract stay unchanged.

Hashes detect accidental corruption; they are not signatures. The manifest is user-controlled executable configuration. Replay evidence only with an adapter command and environment you intend to run.

To turn a counterexample into a regression test, replay it against the fixed application path and require exit `0`. This protects the saved case without asserting universal compatibility.
