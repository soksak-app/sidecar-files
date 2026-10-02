# Features

[한국어](features.ko.md)

- [o] S1 — P1: Build, test and release this sidecar as its own repository. Moved on 2026-10-02 from the soksak core repository (checklist item R1-5-2 there), whose history holds the earlier changes. `make test`, `make build` and `make release` pass on macOS arm64.
- [o] S2 — P1: Test against soksak core 0.0.2. Done on 2026-10-02: the tests resolve `@soksak/plugin-api` from the core tag `v0.0.2`; the sidecar itself is unchanged at 0.0.1, and `make test` passes on macOS arm64.
