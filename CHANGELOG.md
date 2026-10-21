# Changelog

[한국어](CHANGELOG.ko.md)

## Unreleased
- S34: `TestWatchReportsAChangedFileContent` now runs on Linux as well as macOS; Linux execution and policy-reload validation remain pending.
- S31: The process writes one record as its first action, before any setup: the text record `<UTC time> info sidecar files: started` on the standard error, which the host stores as a record of the layer `sidecar`, so the time from the spawn of the process to its first own record is visible. The record goes to the standard error and not to the event file, because the file of the contract starts with `log.open`. A clean run now writes exactly that record on its standard error.

- S30: A failed write stores its pending loss records at the next write. Before, a loss of a failed debug write was stored only when a later debug event arrived, and a loss of a failed main write only when a later main event arrived, so a failed write followed only by Close left a range of seq neither stored nor named.

- S29: A failed write keeps each loss that its batch holds. Each `log.dropped` line of the batch is recorded again with its own channel and reason, and the other lines are recorded as the failed write, one record for each run of consecutive seq. Before, a failed main-file batch that held a debug loss merged the losses into one record of the channel `other`.

- S27: Losses are counted per channel and reason; each `log.dropped` names its channel (`other` for error, warn and info, `debug` for debug and trace) and is stored in the main file. A failed write of the debug file is recorded with the channel debug.

- S28: The writer releases the queue charge of each event it writes or counts as lost, so the queue accepts events again after the writer writes them; before, the queue refused every event after its first bound of events for the life of the process.

- S26: `log.dropped` declares and writes `queued`, the largest queue depth at the refusals that the record counts, and `events.json` declares it as core does; `tests/fixtures/log-contract/base-dropped.json` is copied from the core catalog.

- S25: new commits are authored as `min-0-max <max@blue.tools>`; `.mailmap` maps the former name `min-median-max`, and published commits are not rewritten.

- S24: version 0.0.6 releases the default policy of errors (S23).
- S23: The default policy stores only `error` in every class, before the first policy and while no `log-policy.json` exists, so `warn`, `info`, `debug` and `trace` events are refused. The golden cases `tests/fixtures/policy/cases.json` are copied from core.
- S21: `make hooks` sets `.githooks` as the hook folder, so a commit whose message breaks the commit form or the term rules, or whose records break the record rules, is refused when it is made.
- S20: `make test` runs the record check `soksak-records`, and CI checks the form of each pushed commit message with `soksak-commits`.
- S19: `@soksak/plugin-api` comes from the core release tag `v0.0.11` instead of a commit, and the release workflow builds `sok` from that release.
- F1: the event catalog names the sidecar `files` and declares the `log.*` events in the form of the base, and the writer stores the class of every line, an event outside the catalog as undeclared, and the channel other on log.dropped.
- F2: the writer stores debug and trace in `files-<boot>.debug.jsonl`, which it creates with the first line of its channel; one seq counts across both files and each rotates by its generations.
- S18.5.2: the sidecar reads `log-policy.json` in the parent folder of `--log-dir` when it starts and watches it through the file system events of the platform: a creation, a write, a replacement and a rename put its policy in force, a removal restores the default policy, an empty file waits, and a refused file is reported once as `error: log policy: <text>`. The inotify watch of a regular file reports writes.
- S18.5.1: the writer reads the policy by the rules of core (`events.ParsePolicy`), gates each event of the catalog by its class and level before its fields are built, and stores `log.policy` (first policy and each change) and `log.gated` (exact counts of the events the gate did not build); `tests/fixtures/policy/cases.json` is the copy of the golden cases that core owns.
- S18.4: `events.json` declares the events of the sidecar and `sidecar.json` names it in `events`; `make test` runs the census and the catalog check of `soksak-events` with `census.json`; `@soksak/plugin-api` resolves from a core commit that has the command.
- S18.3: `Serve` stores `sidecar.files.request` (operation, result, `micros`, `error` as `<code>: <text>`), the watch events and `sidecar.files.git` with the `cids` of the request, and each reply line carries the `cids` of its request beside `surface` and `body`.
- S18.2: every goroutine starts through `src/routine`, which records a panic as `sidecar.files.panic` and ends the process with status 2; `main` takes `--log-dir` and stores `sidecar.files.start`, `sidecar.files.failure` and `sidecar.files.exit` instead of only printing the failure.
- S18.1: the event writer (`src/events`) stores `files-<boot>.jsonl` in schema 2 with microsecond `ts_us`, `mono_ns`, `cids`, a bounded queue, `log.dropped`, rotation and a flush with a deadline; the platform reads the monotonic clock.
- S17: the documents and comments call plugins, sidecars and releases by those words.

- F238-d-16: add host-read-only `snapshot`, `expand`, `collapse`, and `select` tree operations. The sidecar owns visible rows, per-path git details, selection, expanded directories, and route generations; each reply carries a full fresh snapshot and echoes request `cids`. Session close, root change, and process restart reset the appropriate in-memory route state.
