# 기능

[English](features.md)

- [o] S1 — P1: 이 sidecar를 자기 repository로 build, test, release한다. 2026-10-02에 soksak core repository(그곳의 checklist 항목 R1-5-2)에서 옮겼으며, 이전 변경 이력은 core에 있다. macOS arm64에서 `make test`, `make build`, `make release`가 통과한다.
- [o] S2 — P1: soksak core 0.0.2에 대해 검사한다. 2026-10-02 완료: test가 core tag `v0.0.2`의 `@soksak/plugin-api`를 쓴다. Sidecar 자체는 0.0.1 그대로이고, macOS arm64에서 `make test`가 통과한다.
