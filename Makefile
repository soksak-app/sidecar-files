# files sidecar repository 의 test, build, release(docs/features.md). release 는 npm pack 으로 sidecar 폴더를 쓴다.
.PHONY: install test build release check-push install-hooks

GO_FLAGS ?=
# The platform key of the release (core docs/spec/installation.md#sidecar-release).
PLATFORM := $(shell node -p '(process.platform === "win32" ? "windows" : process.platform) + "-" + process.arch')

ifeq ($(shell uname -s),Darwin)
PROCESS_IDENTITY_PKGCONFIG = $(shell node -e 'const path=require("node:path");const {createRequire}=require("node:module");const r=createRequire(path.join(process.cwd(),"package.json"));process.stdout.write(path.join(path.dirname(r.resolve("@soksak/native-process-identity-darwin/package.json")),"lib/pkgconfig"))')
PROCESS_IDENTITY_ENV = PKG_CONFIG_PATH="$(PROCESS_IDENTITY_PKGCONFIG)$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH))"
.PHONY: check-process-identity
check-process-identity:
	@node -e 'const fs=require("node:fs");const path=require("node:path");const {createRequire}=require("node:module");const r=createRequire(path.join(process.cwd(),"package.json"));const root=path.dirname(r.resolve("@soksak/native-process-identity-darwin/package.json"));for(const f of ["include/soksak_process_identity.h","lib/libsoksak-process-identity.a","lib/pkgconfig/soksak-process-identity.pc"]){if(!fs.existsSync(path.join(root,f)))throw new Error(`native process identity package is missing ${f}`)}'
test: check-process-identity
build: check-process-identity
endif

# node test 는 sidecar.json 이 선언한 값을 검사하고, sok-check events 는 kit release 의 event source check 와 event catalog 검사다.
# The installation stage (core docs/spec/plugins.md#repositories): it installs the locked dependencies; the tests, checks and packs consume them without installing.
install:
	pnpm install --frozen-lockfile

test:
	$(PROCESS_IDENTITY_ENV) go test ./...
	node --test tests/
	node_modules/.bin/sok-check events --config event-sources.json --catalog events.json
	node_modules/.bin/sok-check docs
	node_modules/.bin/sok-check commands

# sidecar.json 이 가리키는 build/sok-files 를 쓴다.
build:
	$(PROCESS_IDENTITY_ENV) go build $(GO_FLAGS) -o build/sok-files ./src

release: build
	@test -n "$(OUT)" || { echo "make release OUT=<folder>" >&2; exit 2; }
	@mkdir -p $(OUT)
	@# npm pack keeps the execute bit of the executable, which pnpm pack drops.
	file="$$(npm pack --silent --pack-destination $(OUT))" && mv "$(OUT)/$$file" "$(OUT)/$${file%.tgz}-$(PLATFORM).tgz"

# The checks that a push needs: the commit messages and the checklist history of the pushed range, because a published
# commit is kept; CI runs the tests and the other checks on the pushed commits.
# RANGE holds the commits of the pushed refs; the pre-push hook sets it, and it defaults to origin/main..HEAD.
check-push:
	pnpm exec sok-check commits "$(or $(RANGE),origin/main..HEAD)"
	pnpm exec sok-check docs --range "$(or $(RANGE),origin/main..HEAD)"

# Sets this repository to use .githooks, which checks commit messages and records at commit time.
install-hooks:
	@git config core.hooksPath .githooks
