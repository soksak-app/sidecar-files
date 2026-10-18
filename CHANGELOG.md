# Changelog

[한국어](CHANGELOG.ko.md)

## Unreleased

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
