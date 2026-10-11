# 변경 기록

[English](CHANGELOG.md)

## 미배포

- S18.1: event 기록기(`src/events`)가 schema 2의 `files-<boot>.jsonl`을 마이크로초 `ts_us`, `mono_ns`, `cids`, 제한된 queue, `log.dropped`, rotation, deadline이 있는 flush와 함께 저장한다. platform이 monotonic clock을 읽는다.
- S17: 문서와 주석이 plugin, sidecar, release를 그 말로 부른다.
