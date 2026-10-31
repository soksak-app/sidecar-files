# 변경 기록

[English](CHANGELOG.md)

## 미배포

- S49: 0.0.9판은 S48을 담는다.
- S48: `sidecar.json`은 `hostReadOperations`를 선언하지 않는다. core는 F332-4-7을 담은 release부터 그 member를 거부한다.
- S37: v0.0.8이 게시되었고 registry가 이를 올린다.
- F272-files-sidecar: `main`에 push하면 tag가 없을 때 `soksak-release-gate`로 선언된 version을 core release `v0.0.21`의 `sok`와 함께 release한다. core에 대한 의존은 그 release의 pack된 release 파일이다. 어떤 workflow도 core를 checkout하지 않는다.
- S47: 0.0.8판은 core 0.0.21이 필요하다. core 0.0.20에서 이 판을 새로 선택하면 `sidecar.json: unknown field displayName`으로 실패한다. 그 core는 `sidecar.json`의 이전 필드만 읽기 때문이다. 이전 판을 이미 쓰는 설치는 그것을 유지한다. core 0.0.20 설치는 먼저 앱을 0.0.21로 갱신한다.

- S46: sidecar는 process identity package가 process를 display name과 함께 등록하는 core revision에 고정되므로, LaunchServices는 첫 목록부터 sidecar를 display name으로 보여 준다.
- S41-1: `ci.yml`의 macOS runner에서 fresh frozen install이 pin한 native process-identity Git dependency를 resolve하고 그 `prepare`를 실행하며, 사이드카가 그것으로 build되고 test를 통과한다.
- S7: CI와 release workflow가 완료되었다. `release.yml`이 tag에서 release `v0.0.3`부터 `v0.0.6`까지 게시했고, `ci.yml`이 macOS와 Linux에서 `main`을 시험한다.
- S13: S18.5.2가 요구하는 대로 Linux에서 감시하는 정규 파일은 지원하는 감시이며 구현하지 않은 연산이 아니다. S34의 Linux 실행이 이를 확인한다.
- S34: `TestWatchReportsAChangedFileContent`와 정책 reload test가 `ci.yml`의 `test-linux` job(`ubuntu-26.04-arm`)에서 Linux로 통과한다.
- S45: `watch`의 명세가 시작한 감시가 실패하거나 input이 닫힐 때 멈추지 않으면 사이드카가 보내는 `id` 없는 `{error: text}` message를 적는다.
- F275-files-sidecar: `make install`이 고정된 의존성을 설치한다. 시험, 검사, build는 더 이상 설치하지 않는다.
- F274-1-files-sidecar: make target 시험은 중첩된 make의 directory 줄 없이 recipe를 출력한다.
- F274-files-sidecar: pre-push hook은 push가 게시하는 commit message만 검사한다. 시험과 검사는 CI가 실행한다.
- S44: Tracked pnpm workspace policy에서 native process-identity Git
  dependency build만 승인한다. Red: 승인 없는 clean frozen install이 optional
  package payload를 제외해 Files consumer가 `MODULE_NOT_FOUND`로 실패한다.
  Green: 승인된 install이 `prepare: make`를 실행했고 consumer 1/1이 통과하며
  header (526 bytes), static library (3568 bytes), pkg-config file (337 bytes)를
  확인한다. Darwin sidecar package build는 S41-1에서 계속 확인한다.

- S43: Plugin API와 Darwin process-identity package를 동일한 fetch 가능한 public
  core commit에 고정하고 release CLI build도 그 ref를 사용한다. 정규 lock은 두
  package를 version 0.0.21로 resolve한다. Red: 변경 전 manifest consumer는 현재
  SDK validator를 호출하기 전에 필수 `displayName`을 제거했다. Green: consumer가
  전체 `sidecar.json`을 검증하고 manifest owner와 developer-link native-package
  consumer가 각각 1/1 통과하며 SDK event census/catalog 검사는 source 22개와
  event 16개에서 통과한다. 외부 frozen install과 Darwin package build는 S41-1에
  남아 있다. Public v0.0.8은 아직 게시되지 않았다.

- S41-2: 공개 package resolution과 build file로 설치된 Darwin native process-identity package를 검증한다. 소유 consumer 검사와 macOS arm64의 표준 sidecar build가 통과한다. 로컬 override를 누락한 remote-install fixture를 대체하며 로컬 workspace/lock override는 commit하지 않고 외부 Git-ref 설치 acceptance는 S41-1에 유지한다.

- F239-77-files-sidecar: 일치하는 설치 diagnostic cohort로 로컬 schema-3 catalog acceptance를 완료한다. 현재 SDK가 source 22개와 pending rule 0개, event 16개를 검사하며 양쪽 host 설치본의 catalog가 source와 정확히 일치하고 application log audit을 통과한다. Public publication은 별도다.

- F239-77-files-sidecar: Sidecar event catalog를 schema 3 JSON Schema로
  변환하면서 event name, class, level과 field 요구사항을 유지한다. Red: 현재
  core reader가 schema-2 및 compact-field 문제를 보고했다. Green: current-core
  catalog 보고에서 declaration 16개와 emit site 18개에 finding이 없고 focused
  class 및 field-shape Go test 2개가 통과한다. Runtime acceptance에는 schema-3
  core와 package cohort의 새 설치가 필요하다.

- S42: Files sidecar는 `sidecar.json`을 embed하고 host 환경 변수를 읽지 않고 선언된 process identity를 등록한다. Red: 변경 전 executable을 변수 없이 시작하면 변수가 필수라고 보고하고 LaunchServices name을 등록하지 않았다. Green: 변수가 없거나 충돌 값이 있을 때의 startup/live-process test가 통과하고 `make test`와 macOS arm64의 Mach-O arm64 executable build가 통과한다. Release는 게시하지 않았다.
- S41: files sidecar는 `sidecar.json.displayName`을 필수로 받고 `SOKSAK_SIDECAR_DISPLAY_NAME`으로 전달받아 첫 시작 기록을 유지한 채 runtime 초기화 전에 정확한 LaunchServices 이름을 등록한다. Red: 변경 전 진입점은 값이 없어도 시작했고 실제 child의 LaunchServices 항목도 없었다. Green: 두 owning test가 통과하고 scoped Go, manifest, records 검사와 macOS arm64의 local version 0.0.8 build가 통과한다.

- S40: 잘못된 요청 줄은 거부되고 files sidecar는 계속 실행한다. surface를 밝히는 줄은 그 surface에 오류로 답하고, 읽을 수 있는 surface가 없는 줄은 기록한 뒤 건너뛴다. Red: `TestAnInvalidRequestIsRefusedAndTheSidecarKeepsRunning`이 첫 잘못된 줄에서 `serve: invalid request`로 실패했다. Green: 시험이 통과하고 다음 요청에 답한다.
- S39: make-targets test는 자식 make의 환경에서 `MAKEFLAGS`와 `RANGE`를 뺀다. 그래서 pre-push 훅 안에서도 기본 범위를 출력한다. Red: `RANGE`를 설정하면 기본 범위 단정이 실패했다. Green: `RANGE`가 있을 때와 없을 때 모두 시험이 통과한다.
- S38: pre-push 훅이 `make push-check`를 실행한다. 이것은 ci workflow와 같이 `make test`와 push 범위의 commit message 검사를 실행한다. Red: `tests/make-targets.test.mjs`가 없는 recipe와 없는 훅에서 실패했다. Green: 시험 2개 중 2개가 통과하고, `make push-check`가 통과한다.
- S36: `log.dropped` record가 core 카탈로그가 선언한 `channel`과 `queued`를 담는다(F212.1-2-2-3-1). 기록기와 `events.json`은 S26에서 이미 둘을 가졌다. 이 항목은 줄에서 둘을 빼자 golden field test가 실패하는 Red 확인과 그 복구를 기록한다.
- S35: tree test `TestDirectoryDetailFollowsTheRankOfItsDescendantStatuses`가 디렉터리 detail을 자손 status의 순위(deleted, modified, added, renamed, untracked)와 대조한다. code가 순위와 일치하므로 이 test는 pin이다. `renamed`를 `deleted`보다 높이는 변이는 test를 실패시켰다.
- F238-d-16: host가 읽기만 하는 tree 연산 `snapshot`, `expand`, `collapse`, `select`를 추가한다. sidecar가 보이는 행, 경로별 git detail, 선택, 펼친 directory, route generation을 소유하며, 각 응답은 전체 snapshot을 담고 요청의 `cids`를 되돌린다. session 닫기, root 변경, 프로세스 재시작은 해당하는 메모리 내 route 상태를 초기화한다.
- S34: `TestWatchReportsAChangedFileContent`가 macOS와 Linux에서 실행되도록 바뀌었다. Linux 실행과 정책 reload 검증은 미완료다.
- S31: 프로세스가 어떤 setup보다 먼저 첫 동작으로 기록을 한 번 쓴다. 표준 오류에 쓰는 text record `<UTC time> info sidecar files: started`이고, host는 그것을 layer `sidecar`의 record로 저장하므로, 프로세스의 spawn부터 첫 기록까지의 시간이 보인다. 기록은 event file이 아니라 표준 오류로 간다. contract가 정한 file의 첫 줄은 `log.open`이기 때문이다. 깨끗하게 끝나는 실행은 이제 표준 오류에 정확히 이 기록만을 쓴다.

- S30: 실패한 쓰기의 대기 중인 손실 기록을 다음 쓰기에 저장한다. 이전에는 debug 쓰기가 실패한 손실을 다음 debug event가 와야 저장했고, main 쓰기가 실패한 손실도 다음 main event가 와야 저장해서, 실패 뒤 Close만 오면 어떤 seq 범위도 저장도 기록도 되지 않았다.

- S29: 실패한 쓰기는 batch가 담은 loss를 모두 유지한다. batch의 각 `log.dropped` 줄은 자기 channel과 reason으로 다시 기록하고, 나머지 줄은 연속된 seq의 run마다 실패한 쓰기의 기록 하나로 남긴다. 이전에는 debug loss를 담은 main file의 실패한 batch가 loss를 channel `other`의 기록 하나로 합쳤다.

- S27: loss를 channel과 reason별로 센다. 각 `log.dropped`는 channel(error, warn, info는 `other`, debug와 trace는 `debug`)을 담고 main file에 저장된다. debug file의 write 실패도 channel debug로 기록된다.

- S28: writer가 쓰거나 loss로 센 event마다 queue 점유를 해제해, writer가 쓴 뒤 queue가 다시 event를 받아들인다. 이전에는 process 수명 동안 처음 한도 뒤의 모든 event를 거부했다.

- S26: `log.dropped`가 `queued`를 선언하고 기록한다. `queued`는 기록이 세는 거부 시점의 queue 최대 깊이이며, `events.json`은 core와 같이 선언한다. `tests/fixtures/log-contract/base-dropped.json`은 core 카탈로그에서 복사했다.

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

- F238-d-16: host read-only `snapshot`, `expand`, `collapse`, `select` tree operation을 추가한다. 사이드카가 표시 행, 경로별 git detail, 선택, 펼친 디렉터리와 route generation을 소유하고 각 응답은 전체 새 snapshot과 request `cids`를 되돌린다. 세션 종료, root 변경, process 재시작 때 해당 메모리 route 상태를 초기화한다.
