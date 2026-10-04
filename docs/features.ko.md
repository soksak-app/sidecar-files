# 기능

[English](features.md)

- [o] S1 — P1: 이 sidecar를 자기 repository로 build, test, release한다. 2026-10-02에 soksak core repository(그곳의 checklist 항목 R1-5-2)에서 옮겼으며, 이전 변경 이력은 core에 있다. macOS arm64에서 `make test`, `make build`, `make release`가 통과한다.
- [o] S2 — P1: soksak core 0.0.2에 대해 검사한다. 2026-10-02 완료: test가 core tag `v0.0.2`의 `@soksak/plugin-api`를 쓴다. Sidecar 자체는 0.0.1 그대로이고, macOS arm64에서 `make test`가 통과한다.
- [o] S3 — P1: 공용 exposure 검사를 더한 commit으로 옮긴 core tag `v0.0.2`에 대해 검사한다(core R1-5-5). 2026-10-02 완료: lock 파일이 그 commit의 `@soksak/plugin-api`를 쓰고 macOS arm64에서 `make test`가 통과한다.
- [o] S4 — P1: 모든 `closed` 메시지에 core 사이드카 명세가 이제 요구하는 `{surface, closed: true}`나 `{surface, closed: true, error}`로 답한다. 2026-10-03 core G1.4-63-1에서 발견: 사이드카는 실패한 닫기에만 오류 body로 답했고, core host는 그것을 닫힌 표면의 메시지로 버렸다. Red: `TestClosedIsAnsweredAfterItsWatchesEnd`가 실패했다. 마지막 메시지는 목록 답이었고 닫기 답은 오지 않았다. 수정: event는 body나 닫기 답 중 하나를 담고, 사이드카는 감시를 끝낸 뒤 각 `closed`에 답한다. Green: `make test`가 통과한다.
- [o] S5 — P0: core 체크리스트 항목 R2-1을 위해 `@soksak/plugin-api`를 공개된 core 저장소에서 받는다. 2026-10-04 완료: `package.json`은 로컬 폴더 대신 `git+https://github.com/soksak-app/core.git#v0.0.2&path:/packages/plugin-api`를 가리키고, lockfile을 GitHub에서 다시 만들었으며, macOS arm64에서 `make test`가 통과한다.
- [o] S6 — P0: core 체크리스트 항목 R2-2-2를 위해 version 0.0.3을 선언한다. 2026-10-04 완료: `package.json`은 0.0.1 대신 0.0.3을 선언하고, `@soksak/plugin-api`는 tag `v0.0.3`이 게시될 때까지 core commit 22162a74에서 받는다. macOS 26 arm64에서 `make test`가 통과한다.
