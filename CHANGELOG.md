# Changelog

[한국어](CHANGELOG.ko.md)

## Unreleased

- S18.4: `events.json` declares the events of the sidecar and `sidecar.json` names it in `events`; `make test` runs the census and the catalog check of `soksak-events` with `census.json`; `@soksak/plugin-api` resolves from a core commit that has the command.
- S18.3: `Serve` stores `sidecar.files.request` (operation, result, `micros`, `error` as `<code>: <text>`), the watch events and `sidecar.files.git` with the `cids` of the request, and each reply line carries the `cids` of its request beside `surface` and `body`.
- S18.2: every goroutine starts through `src/routine`, which records a panic as `sidecar.files.panic` and ends the process with status 2; `main` takes `--log-dir` and stores `sidecar.files.start`, `sidecar.files.failure` and `sidecar.files.exit` instead of only printing the failure.
- S18.1: the event writer (`src/events`) stores `files-<boot>.jsonl` in schema 2 with microsecond `ts_us`, `mono_ns`, `cids`, a bounded queue, `log.dropped`, rotation and a flush with a deadline; the platform reads the monotonic clock.
- S17: the documents and comments call plugins, sidecars and releases by those words.
