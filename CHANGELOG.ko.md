# 변경 기록

[English](CHANGELOG.md)

## 미배포

- S18.5.1: 기록기가 core의 규칙대로 정책을 읽고(`events.ParsePolicy`), catalog의 각 event를 field를 만들기 전에 class와 level로 gate에 통과시키며, `log.policy`(첫 정책과 모든 변경)와 `log.gated`(gate가 만들지 않은 event의 정확한 수)를 저장한다. `tests/fixtures/policy/cases.json`은 core가 소유한 골든 사례의 사본이다.
- S18.4: `events.json`이 사이드카의 event를 선언하고 `sidecar.json`이 `events`로 가리킨다. `make test`가 `census.json`으로 `soksak-events`의 census와 카탈로그 검사를 실행한다. `@soksak/plugin-api`는 이 명령이 있는 core commit에서 받는다.
- S18.3: `Serve`가 `sidecar.files.request`(operation, 결과, `micros`, `<code>: <text>`인 `error`), 감시 event, `sidecar.files.git`를 요청의 `cids`와 함께 저장하고, 각 답 줄이 `surface`와 `body` 옆에 요청의 `cids`를 싣는다.
- S18.2: 모든 goroutine이 `src/routine`으로 시작하며, 이 입구가 panic을 `sidecar.files.panic`으로 기록하고 status 2로 프로세스를 끝낸다. `main`은 `--log-dir`를 받고 실패를 출력만 하지 않고 `sidecar.files.start`, `sidecar.files.failure`, `sidecar.files.exit`로 저장한다.
- S18.1: event 기록기(`src/events`)가 schema 2의 `files-<boot>.jsonl`을 마이크로초 `ts_us`, `mono_ns`, `cids`, 제한된 queue, `log.dropped`, rotation, deadline이 있는 flush와 함께 저장한다. platform이 monotonic clock을 읽는다.
- S17: 문서와 주석이 plugin, sidecar, release를 그 말로 부른다.
