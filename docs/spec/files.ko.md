# 파일 사이드카

[English](files.md)


`@soksak/sidecar-files`(이 저장소)는 `make build`로 `build/soksak-files`를 빌드하고 세션의 `root` 안의 디렉터리를 나열하고 감시한다. 코드는 `src/`에 있다: 진입점 `src/main.go`, 패키지 `src/files`의 프로토콜, `src/platform/platform.go`를 통해 등록되는 `src/platform/{darwin,linux,windows}/`의 디렉터리 감시. macOS는 디렉터리나 일반 파일에 kqueue `EVFILT_VNODE` 필터를 걸어 감시하고, Linux는 디렉터리와 일반 파일을 inotify로 감시하고, Windows는 `watching directories is not implemented on windows`로 실패한다. 세션은 감시하는 경로만 상태로 갖는다. 사이드카는 사이드카 출력 줄 한도와 같은 67108864 byte까지의 요청 줄을 읽고, 답을 HTML escape 없이 쓴다. JSON이 최대 여섯 배로 escape한 8 MiB 텍스트와 32 MiB bytes의 base64는 그 한도 안에 있다.

| 요청 본문 | 답 본문 |
| --- | --- |
| `{operation: "list", id, path}` | `{id, entries: [{name, directory}]}`: `root/path`의 항목. 디렉터리가 먼저 오고 각 묶음은 이름순이다. `path`는 `root` 기준 상대 경로이며 `""`는 `root` 자신이다 |
| `{operation: "watch", id, paths}` | `{id}`: 세션이 감시하는 경로를 `paths`(`root` 기준 상대 경로, `list`와 같이 검사)로 바꾼다. 빈 목록은 감시를 멈춘다. 그 뒤 감시하는 디렉터리의 항목이 생기거나 지워지거나 이름이 바뀔 때와, 감시하는 일반 파일의 내용이 쓰이거나 늘어날 때(macOS 파일의 kqueue `NOTE_WRITE`나 `NOTE_EXTEND`; Linux 파일의 inotify 쓰기, 삭제, 이동 event) 사이드카는 `id` 없이 `{changed: path}`를 보낸다 |
| `{operation: "snapshot", id}` | `{id, tree: {generation, entries, selected}}`: 이 sidecar session과 project root의 표시 트리 전체. project root는 generation 1에서 펼쳐지고 처음에는 선택이 없다 |
| `{operation: "expand", id, generation, path}` | `{id, tree: {generation, entries, selected}}`: `path`의 나열된 디렉터리를 펼쳐 자식을 나열하고 펼친 모든 디렉터리를 감시한다 |
| `{operation: "collapse", id, generation, path}` | `{id, tree: {generation, entries, selected}}`: 디렉터리와 펼친 자손을 접고 보이는 행을 제거하며 접힌 디렉터리 안의 선택을 지운다 |
| `{operation: "select", id, generation, path}` | `{id, tree: {generation, entries, selected}}`: 현재 보이는 경로를 선택한다. 빈 `path`는 선택을 지운다 |

`tree.entries`는 표시 중인 행의 평탄한 깊이 우선 목록 `{path, name, directory, depth, expanded, selected, detail?}`이다. 파일 detail은 git 상태다. 디렉터리 detail은 자손 상태 중 우선순위가 가장 높은 항목이며 순서는 `deleted`, `modified`, `added`, `renamed`, `untracked`, 동순위는 경로 순이다. `generation`은 sidecar session(`surface`)과 project root에 범위를 둔다. 오래된 generation이나 `snapshot` 이전 tree 작업은 실패한다. 새 root는 generation을 새로 시작하고 펼침과 선택을 초기화한다. `closed`는 route 상태를 지우며 sidecar 재시작은 모든 route를 초기화한다. Reply envelope는 기존 sidecar message contract에 따라 request `cids`를 되돌린다. `sidecar.json`은 이 네 host operation만 허용하며 `snapshot`은 session 디렉터리 감시도 갱신한다. 새로 나열한 부모에 펼쳐진 디렉터리가 더 이상 없으면 오래된 경로를 정리한다. 부모에 여전히 존재하는 펼친 경로에서 `List(root, path)`가 실패하면 operation은 `{id, error}`를 반환해야 하며 디렉터리를 조용히 버리거나 성공 snapshot을 내보내면 안 된다.
| `{operation: "git", id}` | `{id, entries: [{path, status}]}`: `root`에서 `git status --porcelain=v1 -z --untracked-files=all`을 실행하고 각 항목을 `added`, `deleted`, `modified`, `renamed`, `untracked` 중 하나와 저장소 최상위 기준 경로를 `root` 기준으로 바꾼 경로로 옮긴다. `root` 밖의 항목은 뺀다. `root`가 git 저장소 안에 있지 않거나 git이 설치되지 않았으면 `entries`는 비어 있고, 다른 git 실패는 오류다 |
| `{operation: "read", id, path}` | `{id, text, version, newline, bom}`: 일반 파일 `root/path`의 내용을 텍스트로 돌려준다. `version`은 파일 byte의 소문자 16진수 SHA-256이고, `newline`은 `lf`, `crlf`, `cr`, `mixed`, `none`(줄바꿈 없음) 중 하나이며, `bom`은 파일이 UTF-8 byte order mark로 시작하면 `true`이고 `text`는 그것을 담지 않는다. 올바른 UTF-8이 아닌 파일은 `not UTF-8 text: <path>`로, 8388608 byte를 넘는 파일은 내용을 읽기 전에 `file is <n> bytes, above the 8388608-byte limit: <path>`로, 일반 파일이 아닌 경로는 `not a regular file: <path>`로 실패한다 |
| `{operation: "write", id, path, text, expect, bom}` | `{id, version}`: `bom`이 `true`면 UTF-8 byte order mark 뒤에 `text`를 `root/path`에 제자리로 쓰고, 쓴 byte의 SHA-256을 돌려준다. `expect`가 version이면 사이드카는 기존 일반 파일을 자르지 않고 쓰기로 열어 같은 descriptor로 읽고, 그 SHA-256이 `expect`와 다르면 쓰지 않고 `changed on disk: <path>`로 실패한다. 같으면 처음부터 전체 내용을 쓰고, 파일을 그 길이로 자르고, 디스크에 동기화한다. 제자리 쓰기는 파일의 inode, mode, 확장 속성, 접근 제어 목록, hard link, symbolic link의 대상을 유지한다. 첫 byte를 쓴 뒤의 실패는 `write incomplete: <path>: <error>`로 실패하며 그때 파일은 내용의 일부를 담을 수 있다. `expect`가 `null`이면 사이드카는 process umask로 가린 mode 0666의 새 파일을 만들고, 경로가 있으면 `exists: <path>`로 실패한다. 8388608 byte를 넘는 `text`는 큰 `read`와 같이 실패한다 |
| `{operation: "readBytes", id, path}` | `{id, data, version}`: 정규 파일 `root/path`의 bytes를 padding이 있는 표준 base64로 `data`에, 그 SHA-256을 `version`에 담는다. 33554432 bytes를 넘는 파일은 내용을 읽기 전에 `file is <n> bytes, above the 33554432-byte limit: <path>`로 실패하고, 정규 파일이 아닌 경로는 `not a regular file: <path>`로 실패한다 |
| `{operation: "writeBytes", id, path, data, expect}` | `{id, version}`: `data`가 padding이 있는 표준 base64로 담은 bytes를 `write`의 제자리, `expect`, 생성 규칙으로 `root/path`에 쓰고 그 SHA-256을 반환한다. 표준 base64가 아닌 `data`는 `writeBytes data is not base64`로 실패하고, 33554432 bytes를 넘는 decode한 bytes는 큰 `readBytes`처럼 실패한다 |
| `closed` | 세션의 감시를 멈춘 뒤 끝나면 `{surface, closed: true}`로 답하고, 멈추지 못하면 `{surface, closed: true, error}`로 답한다. 이 답은 core의 `docs/spec/sidecars.md#messages`가 정한 닫기 답이다 |
| 모든 실패 | `{id, error}`: `root`가 없거나, `path`가 절대 경로이거나 (심볼릭 링크를 따라간 뒤) `root`를 벗어나거나, 디렉터리를 읽을 수 없는 경우 |
