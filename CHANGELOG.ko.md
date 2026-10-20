# 변경 기록

[English](CHANGELOG.md)

## 미배포

- S25: 새 커밋의 작성자는 `min-0-max <max@blue.tools>`다. `.mailmap`이 이전 이름 `min-median-max`를 잇고, 게시된 커밋은 다시 쓰지 않는다.

- S24: version 0.0.6이 오류의 기본 정책(S23)을 릴리즈한다.
- S23: 기본 정책은 모든 class에서 `error`만 저장하며, 첫 정책 전과 `log-policy.json`이 없는 동안 적용된다. 그러므로 `warn`, `info`, `debug`, `trace` event는 거부된다. 골든 사례 `tests/fixtures/policy/cases.json`은 core에서 복사했다.
- S21: `make hooks`가 `.githooks`를 hook 폴더로 정해, commit message의 형식이나 용어 규칙, record 규칙을 어긴 commit이 만들어질 때 거부된다.
- S20: `make test`가 record 검사 `soksak-records`를 실행하고, CI가 push된 각 commit message의 형식을 `soksak-commits`로 검사한다.
- S19: `@soksak/plugin-api`를 commit 대신 core release tag `v0.0.11`에서 받고, release workflow가 그 release에서 `sok`를 build한다.
- S18.5.2: 사이드카가 시작할 때 `--log-dir`의 부모 폴더의 `log-policy.json`을 읽고 플랫폼의 파일 시스템 event로 감시한다. 생성, 쓰기, 교체, 이름 변경은 그 정책을 적용하고, 삭제는 기본 정책을 되살리며, 빈 파일은 기다리고, 거부된 파일은 `error: log policy: <글>`로 한 번 보고한다. 정규 파일에 대한 inotify 감시가 쓰기를 알린다.
- S18.5.1: 기록기가 core의 규칙대로 정책을 읽고(`events.ParsePolicy`), catalog의 각 event를 field를 만들기 전에 class와 level로 gate에 통과시키며, `log.policy`(첫 정책과 모든 변경)와 `log.gated`(gate가 만들지 않은 event의 정확한 수)를 저장한다. `tests/fixtures/policy/cases.json`은 core가 소유한 골든 사례의 사본이다.
- S18.4: `events.json`이 사이드카의 event를 선언하고 `sidecar.json`이 `events`로 가리킨다. `make test`가 `census.json`으로 `soksak-events`의 census와 카탈로그 검사를 실행한다. `@soksak/plugin-api`는 이 명령이 있는 core commit에서 받는다.
- S18.3: `Serve`가 `sidecar.files.request`(operation, 결과, `micros`, `<code>: <text>`인 `error`), 감시 event, `sidecar.files.git`를 요청의 `cids`와 함께 저장하고, 각 답 줄이 `surface`와 `body` 옆에 요청의 `cids`를 싣는다.
- S18.2: 모든 goroutine이 `src/routine`으로 시작하며, 이 입구가 panic을 `sidecar.files.panic`으로 기록하고 status 2로 프로세스를 끝낸다. `main`은 `--log-dir`를 받고 실패를 출력만 하지 않고 `sidecar.files.start`, `sidecar.files.failure`, `sidecar.files.exit`로 저장한다.
- S18.1: event 기록기(`src/events`)가 schema 2의 `files-<boot>.jsonl`을 마이크로초 `ts_us`, `mono_ns`, `cids`, 제한된 queue, `log.dropped`, rotation, deadline이 있는 flush와 함께 저장한다. platform이 monotonic clock을 읽는다.
- S17: 문서와 주석이 plugin, sidecar, release를 그 말로 부른다.
