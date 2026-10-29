# files sidecar repository 의 test, build, release(docs/features.md). release 는 core 의 sok 를 쓴다.
.PHONY: install test build release push-check hooks

GO_FLAGS ?=
SOK ?= sok

ifeq ($(shell uname -s),Darwin)
PROCESS_IDENTITY_PKGCONFIG = $(shell node -e 'const path=require("node:path");const {createRequire}=require("node:module");const r=createRequire(path.join(process.cwd(),"package.json"));process.stdout.write(path.join(path.dirname(r.resolve("@soksak/native-process-identity-darwin/package.json")),"lib/pkgconfig"))')
PROCESS_IDENTITY_ENV = PKG_CONFIG_PATH="$(PROCESS_IDENTITY_PKGCONFIG)$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH))"
.PHONY: process-identity-check
process-identity-check:
	@node -e 'const fs=require("node:fs");const path=require("node:path");const {createRequire}=require("node:module");const r=createRequire(path.join(process.cwd(),"package.json"));const root=path.dirname(r.resolve("@soksak/native-process-identity-darwin/package.json"));for(const f of ["include/soksak_process_identity.h","lib/libsoksak-process-identity.a","lib/pkgconfig/soksak-process-identity.pc"]){if(!fs.existsSync(path.join(root,f)))throw new Error(`native process identity package is missing ${f}`)}'
test: process-identity-check
build: process-identity-check
endif

# node test 는 sidecar.json 을 package.json 이 정한 core commit 의 @soksak/plugin-api 로 검사하고, soksak-events 는 같은 commit 의 census 와 event 카탈로그 검사다.
# The installation stage (core docs/spec/plugins.md#repositories): it installs the locked dependencies; the tests, checks and packs consume them without installing.
install:
	pnpm install --frozen-lockfile

test:
	$(PROCESS_IDENTITY_ENV) go test ./...
	node --test tests/
	node_modules/.bin/soksak-events --census census.json --catalog events.json
	node_modules/.bin/soksak-records

# sidecar.json 이 가리키는 build/soksak-files 를 쓴다.
build:
	$(PROCESS_IDENTITY_ENV) go build $(GO_FLAGS) -o build/soksak-files ./src

release: build
	@test -n "$(OUT)" || { echo "make release OUT=<folder>" >&2; exit 2; }
	$(SOK) sidecar release . $(OUT)

# The check that a push needs: the commit messages of the pushed range, because a published commit message is kept; CI runs
# the tests and checks on the pushed commits.
# RANGE holds the commits of the pushed refs; the pre-push hook sets it, and it defaults to origin/main..HEAD.
push-check:
	pnpm exec soksak-commits "$(or $(RANGE),origin/main..HEAD)"

# Sets this repository to use .githooks, which checks commit messages and records at commit time.
hooks:
	@git config core.hooksPath .githooks
