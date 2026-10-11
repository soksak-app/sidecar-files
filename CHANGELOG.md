# Changelog

[한국어](CHANGELOG.ko.md)

## Unreleased

- S18.1: the event writer (`src/events`) stores `files-<boot>.jsonl` in schema 2 with microsecond `ts_us`, `mono_ns`, `cids`, a bounded queue, `log.dropped`, rotation and a flush with a deadline; the platform reads the monotonic clock.
- S17: the documents and comments call plugins, sidecars and releases by those words.
