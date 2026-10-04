# 기능

[English](features.md)

- [o] S1 — P1: 이 sidecar를 자기 repository로 build, test, release한다. 2026-10-02에 soksak core repository(그곳의 checklist 항목 R1-5-2)에서 옮겼으며, 이전 변경 이력은 core에 있다. macOS arm64에서 `make test`, `make build`, `make release`가 통과한다.
- [o] S2 — P1: soksak core 0.0.2에 대해 검사한다. 2026-10-02 완료: test가 core tag `v0.0.2`의 `@soksak/plugin-api`를 쓴다. Sidecar 자체는 0.0.1 그대로이고, macOS arm64에서 `make test`가 통과한다.
- [o] S3 — P1: 공용 exposure 검사를 더한 commit으로 옮긴 core tag `v0.0.2`에 대해 검사한다(core R1-5-5). 2026-10-02 완료: lock 파일이 그 commit의 `@soksak/plugin-api`를 쓰고 macOS arm64에서 `make test`가 통과한다.
- [o] S4 — P1: 모든 `closed` 메시지에 core 사이드카 명세가 이제 요구하는 `{surface, closed: true}`나 `{surface, closed: true, error}`로 답한다. 2026-10-03 core G1.4-63-1에서 발견: 사이드카는 실패한 닫기에만 오류 body로 답했고, core host는 그것을 닫힌 표면의 메시지로 버렸다. Red: `TestClosedIsAnsweredAfterItsWatchesEnd`가 실패했다. 마지막 메시지는 목록 답이었고 닫기 답은 오지 않았다. 수정: event는 body나 닫기 답 중 하나를 담고, 사이드카는 감시를 끝낸 뒤 각 `closed`에 답한다. Green: `make test`가 통과한다.
- [o] S5 — P0: core 체크리스트 항목 R2-1을 위해 `@soksak/plugin-api`를 공개된 core 저장소에서 받는다. 2026-10-04 완료: `package.json`은 로컬 폴더 대신 `git+https://github.com/soksak-app/core.git#v0.0.2&path:/packages/plugin-api`를 가리키고, lockfile을 GitHub에서 다시 만들었으며, macOS arm64에서 `make test`가 통과한다.
- [o] S6 — P0: core 체크리스트 항목 R2-2-2를 위해 version 0.0.3을 선언한다. 2026-10-04 완료: `package.json`은 0.0.1 대신 0.0.3을 선언하고, `@soksak/plugin-api`는 tag `v0.0.3`이 게시될 때까지 core commit 22162a74에서 받는다. macOS 26 arm64에서 `make test`가 통과한다.
- [~] S7 — P0: core checklist 항목 R2-5-2를 위해 CI와 release workflow를 둔다. `ci.yml`은 macOS runner에서 이 저장소의 테스트를 실행하고, tag `v*`는 `release.yml`을 실행해 같은 core tag에서 `sok`을 빌드하고 release profile의 sidecar archive를 게시하며, `.node-version`과 `packageManager`가 workflow가 설치하는 Node.js와 pnpm 버전을 선언한다. 2026-10-04 진행: `ci.yml`의 build-machine rehearsal이 macOS에서 통과했다. `release.yml`의 rehearsal에는 tag가 필요하다.
- [o] S8 — P2: core checklist 항목 R2-7-2를 위해 Linux에서 inotify로 디렉터리를 감시한다. Linux `Watch`는 macOS 감시처럼 디렉터리 안에서 항목이 생기거나 지워지거나 이름이 바뀔 때와 디렉터리 자신이 지워지거나 옮겨질 때를 알리고, stop이 pipe를 닫으면 끝나며, `ci.yml`은 Linux runner에서도 `make test`를 실행한다. 2026-10-04 완료: `src/platform/linux/watch.go`가 지원하지 않던 Linux `Watch`를 대신한다. 검증: `TestWatchReportsAChangedDirectory`를 포함해 `ci.yml`의 build-machine Linux replay가 통과했고(run `20261004-202154-547085`, Ubuntu 26.04 arm64), macOS 26 arm64에서 `make test`가 통과한다.
