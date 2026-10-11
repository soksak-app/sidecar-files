# files sidecar repository 의 test, build, release(docs/features.md). release 는 core 의 sok 를 쓴다.
.PHONY: test build release

GO_FLAGS ?=
SOK ?= sok

# node test 는 sidecar.json 을 package.json 이 정한 core commit 의 @soksak/plugin-api 로 검사하고, soksak-events 는 같은 commit 의 census 와 event 카탈로그 검사다.
node_modules: package.json
	pnpm install
	touch node_modules

test: node_modules
	go test ./...
	node --test tests/
	node_modules/.bin/soksak-events --census census.json --catalog events.json

# sidecar.json 이 가리키는 build/soksak-files 를 쓴다.
build:
	go build $(GO_FLAGS) -o build/soksak-files ./src

release: build
	@test -n "$(OUT)" || { echo "make release OUT=<folder>" >&2; exit 2; }
	$(SOK) sidecar release . $(OUT)
