# Features

[한국어](features.ko.md)

- [o] S1 — P1: Build, test and release this sidecar as its own repository. Moved on 2026-10-02 from the soksak core repository (checklist item R1-5-2 there), whose history holds the earlier changes. `make test`, `make build` and `make release` pass on macOS arm64.
- [o] S2 — P1: Test against soksak core 0.0.2. Done on 2026-10-02: the tests resolve `@soksak/plugin-api` from the core tag `v0.0.2`; the sidecar itself is unchanged at 0.0.1, and `make test` passes on macOS arm64.
- [o] S3 — P1: Test against the core tag `v0.0.2` after it moved to the commit that adds the shared exposure check (core R1-5-5). Done on 2026-10-02: the lock file resolves `@soksak/plugin-api` from that commit and `make test` passes on macOS arm64.
- [o] S4 — P1: Answer every `closed` message with `{surface, closed: true}` or `{surface, closed: true, error}`, as the core sidecar specification now requires. Found on 2026-10-03 by core G1.4-63-1: the sidecar answered only a failed close, with an error body that the core hosts discarded as a message for a closed surface. Red: `TestClosedIsAnsweredAfterItsWatchesEnd` failed: the last message was the list answer and no close answer followed. Correction: an event carries either a body or the close answer, and the sidecar answers each `closed` after its watches end. Green: `make test` passes.
- [o] S5 — P0: Resolve `@soksak/plugin-api` from the published core repository, for the core checklist item R2-1. Done on 2026-10-04: `package.json` names `git+https://github.com/soksak-app/core.git#v0.0.2&path:/packages/plugin-api` instead of a local folder, the lockfile was regenerated from GitHub, and `make test` passes on macOS arm64.
