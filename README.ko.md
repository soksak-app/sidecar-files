# Files sidecar

[English](README.md)

`@soksak/sidecar-files`: 줄 단위 JSON으로 프로젝트 폴더 안의 디렉터리를 나열하고 감시하며, text 파일을 읽고 쓰고 감시한다. 프로토콜과 host 계약은 soksak core spec(`docs/spec/sidecars.md`)이 정한다.

```sh
make test                 # test
make build                # sidecar.json이 가리키는 실행 파일
make release OUT=<folder> SOK=<core>/target/debug/sok   # 이 platform의 release
```

Checklist는 [docs/features.md](docs/features.md)다.
