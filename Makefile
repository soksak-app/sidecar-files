# files sidecar repository 의 test, build, release(docs/features.md). release 는 core 의 sok 를 쓴다.
.PHONY: test build release

GO_FLAGS ?=
SOK ?= sok

# node test 는 sidecar.json 을 core tag 의 @soksak/plugin-api 로 검사한다.
node_modules: package.json
	pnpm install
	touch node_modules

test: node_modules
	go test ./...
	node --test tests/

# sidecar.json 이 가리키는 build/soksak-files 를 쓴다.
build:
	go build $(GO_FLAGS) -o build/soksak-files ./src

release: build
	@test -n "$(OUT)" || { echo "make release OUT=<folder>" >&2; exit 2; }
	$(SOK) sidecar release . $(OUT)
