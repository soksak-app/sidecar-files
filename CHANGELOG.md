# Changelog

[한국어](CHANGELOG.ko.md)

## Unreleased

- S7: The CI and release workflows are complete: `release.yml` published the releases `v0.0.3` to `v0.0.6` from their tags, and `ci.yml` tests `main` on macOS and Linux.
- S13: A watched regular file on Linux is a supported watch, as S18.5.2 requires, and not an unimplemented operation; the Linux run of S34 confirms it.
- S34: `TestWatchReportsAChangedFileContent` and the policy reload tests pass on Linux in the `test-linux` job of `ci.yml` (`ubuntu-26.04-arm`).
- S45: The specification of `watch` states the message `{error: text}` without `id` that the sidecar sends when a watch fails after it started or does not stop as the input closes.
- F275-files-sidecar: `make install` installs the locked dependencies; the tests, checks and builds no longer install.
- F274-1-files-sidecar: The make target test prints a recipe without the directory lines of a nested make.
- F274-files-sidecar: The pre-push hook checks only the commit messages that a push publishes; CI runs the tests and checks.
- S44: Approve only the native process-identity Git dependency build in the
  tracked pnpm workspace policy. Red: without approval, a clean frozen install
  omitted the optional package payload and the Files consumer failed with
  `MODULE_NOT_FOUND`. Green: the approved install ran `prepare: make`; the
  consumer passed 1/1 and found the header (526 bytes), static library
  (3568 bytes) and pkg-config file (337 bytes). The Darwin sidecar package
  build remains open under S41-1.

- S43: Pin the plugin API and Darwin process-identity package to the same
  fetchable public core commit and use it for the release CLI build. The
  canonical lock resolves both as version 0.0.21. Red: the unchanged manifest
  consumer removed the required `displayName` before calling the current SDK
  validator. Green: the consumer validates the full `sidecar.json`; the
  manifest owner and developer-linked native-package consumer pass 1/1 each,
  and the SDK event census/catalog check passes for 22 sources and 16 events.
  The external frozen install and Darwin package build remain under S41-1; public
  v0.0.8 remains unpublished.

- S41-2: Verify the installed Darwin native process-identity package through public package resolution and its build files. The owning consumer passes and the standard sidecar build succeeds on macOS arm64. Replace the remote-install fixture that omitted local overrides; keep local workspace/lock overrides uncommitted and external Git-ref installation acceptance open under S41-1.

- F239-77-files-sidecar: Complete local schema-3 catalog acceptance with the matching installed diagnostic cohort. The current SDK checks 22 sources with zero pending rules and 16 events; both host installations contain the exact source catalog and pass application log audits. Public publication remains separate.

- F239-77-files-sidecar: Convert the sidecar event catalog to schema 3 JSON
  Schema while preserving event names, classes, levels and field requirements.
  Red: the current core reader reported schema-2 and compact-field findings.
  Green: current-core catalog reporting has zero findings for 16 declarations
  and 18 emit sites; the focused class and field-shape Go tests pass 2/2.
  Runtime acceptance requires a fresh install with the schema-3 core and package
  cohort.

- S42: The Files sidecar embeds `sidecar.json` and registers its declared process identity without reading a host environment variable. Red: the live-process test started the unchanged executable without that variable, which reported it as required and had no LaunchServices name. Green: the startup and live-process tests pass with the variable absent and with a conflicting value; `make test` passes, and the sidecar builds as a Mach-O arm64 executable on macOS arm64. No release was published.
- S41: The files sidecar requires `sidecar.json.displayName`, receives it as `SOKSAK_SIDECAR_DISPLAY_NAME`, and registers the exact LaunchServices name before runtime initialization while retaining its first start record. Red: the unchanged entry point accepted a missing value, and a real child had no LaunchServices entry. Green: both owning tests pass; scoped Go, manifest, records checks and the local version 0.0.8 build pass on macOS arm64.

- S40: An invalid request line is refused and the files sidecar keeps running: a line that names its surface is answered with its error on that surface, and a line without a readable surface is logged and skipped. Red: the test `TestAnInvalidRequestIsRefusedAndTheSidecarKeepsRunning` failed with `serve: invalid request` when the sidecar exited on the first invalid line. Green: the test passes and the next request is answered.
- S39: The make-targets test clears `MAKEFLAGS` and `RANGE` from the environment of the child make, so the default range is printed inside the pre-push hook. Red: with `RANGE` set, the default range assertion failed. Green: the test passes with and without `RANGE`.
- S38: The pre-push hook runs `make push-check`, which runs `make test` and the commit message check of the pushed range, as the ci workflow does. Red: `tests/make-targets.test.mjs` failed on the missing recipe and the missing hook. Green: 2 of 2 tests pass, and `make push-check` passes.
- S36: The `log.dropped` record carries `channel` and `queued` as the core catalog declares them (F212.1-2-2-3-1). The emitter and `events.json` already had both from S26; this entry records the Red check, in which removing them from the emitted line failed the golden field tests, and its restoration.
- S35: The tree test `TestDirectoryDetailFollowsTheRankOfItsDescendantStatuses` checks the directory detail against the rank of its descendant statuses (deleted, modified, added, renamed, untracked). The code matches the rank, so the test is a pin. A mutation that ranked `renamed` above `deleted` failed the test.
- F238-d-16: add the host-read-only `snapshot`, `expand`, `collapse` and `select` tree operations. The sidecar owns the visible rows, per-path git details, selection, expanded directories and route generations; each reply carries a full snapshot and echoes the request `cids`. Closing a session, changing the root and restarting the process reset the matching in-memory route state.
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
