.PHONY: proto-gen plugin-sdk plugin-zips build test test-image test-shell lint sec-scan clean \
	ci-fast ci-fast-vet ci-fast-vuln ci-fast-build ci-fast-sec \
	ci-local ci-local-lint ci-local-test ci-local-frontend ci-arch pre-push hooks \
	migrate-up migrate-down migrate-version setup dev dev-down dev-logs \
	infra frontend-typecheck \
	mobile-build mobile-install mobile mobile-go \
	mutation-test docker-image verify-deploy

BINARY=evey
DC=docker compose --env-file .env.dev

# ── Build identity ─────────────────────────────────────────────────────────────
# The commit and build date stamped into the binary and into the image labels.
# Both fall back to the literal "unknown" rather than to an empty string: an
# empty --build-arg OVERRIDES the Dockerfile default and produces an image that
# reports "" as its commit - a build that lies. `$(or ...)` collapses the empty
# `git rev-parse` output (outside a checkout, or without git installed) onto the
# honest value, and the Dockerfile rejects anything else. See scripts/verify-deploy.sh.
GIT_COMMIT := $(or $(shell git rev-parse HEAD 2>/dev/null),unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
# `/` is legal in a branch name and illegal in a docker tag - substitute it.
VERSION    := $(subst /,-,$(or $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null),dev))
GO_LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT) -X main.buildDate=$(BUILD_DATE)

# ── Build ──────────────────────────────────────────────────────────────────────

proto-gen:
	buf generate

plugin-sdk:
	@echo "Generating plugin SDK stubs..."
	@# Python - throwaway container (the evey image dropped grpcio-tools when
	@# install-time stub copy replaced runtime protoc generation)
	docker run --rm -v $(PWD):/work python:3.12-slim sh -c 'pip install -q grpcio-tools >/dev/null && mkdir -p /tmp/sdk_gen && python -m grpc_tools.protoc --proto_path=/work/proto --python_out=/tmp/sdk_gen --grpc_python_out=/tmp/sdk_gen /work/proto/plugin/v1/plugin_api.proto && cp /tmp/sdk_gen/plugin/v1/plugin_api_pb2.py /tmp/sdk_gen/plugin/v1/plugin_api_pb2_grpc.py /work/sdk/python/'
	sed -i 's/from plugin\.v1 import /import /g' sdk/python/plugin_api_pb2_grpc.py
	@# Node.js - proto-loader (runtime), just copy proto
	cp proto/plugin/v1/plugin_api.proto sdk/nodejs/
	@# PHP - needs protoc + grpc_php_plugin (see Dockerfile.plugins.example)
	cp proto/plugin/v1/plugin_api.proto sdk/php/
	@# Ruby - needs grpc-tools gem (see Dockerfile.plugins.example)
	cp proto/plugin/v1/plugin_api.proto sdk/ruby/
	@# Java/Kotlin - needs protoc-gen-grpc-java (see Dockerfile.plugins.example)
	mkdir -p sdk/java
	cp proto/plugin/v1/plugin_api.proto sdk/java/
	@# Swift - needs protoc-gen-swift + protoc-gen-grpc-swift
	mkdir -p sdk/swift
	cp proto/plugin/v1/plugin_api.proto sdk/swift/
	@# Dart - needs protoc-gen-dart
	mkdir -p sdk/dart
	cp proto/plugin/v1/plugin_api.proto sdk/dart/
	@echo "Done - sdk/ updated"

plugin-zips:
	@./scripts/build-test-plugins.sh

build:
	go build -ldflags="$(GO_LDFLAGS)" -o bin/$(BINARY) ./cmd/evey

# docker-image: local image build carrying the SAME stamps CI applies, so a
# hand-built image identifies itself exactly like a published one. Never pushes.
docker-image:
	docker build \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg COMMIT=$(GIT_COMMIT) \
	  --build-arg BUILD_DATE=$(BUILD_DATE) \
	  --build-arg RUNTIMES=$(or $(RUNTIMES),lean) \
	  -t evey:$(VERSION) .

# verify-deploy: ask a running server which commit it is on and compare it to
# this checkout's HEAD. Exits non-zero on a mismatch AND on "cannot tell".
# Usage: make verify-deploy URL=https://example.tld [COMMIT=<sha>]
verify-deploy:
	@./scripts/verify-deploy.sh $(if $(URL),--url $(URL),) $(if $(COMMIT),--commit $(COMMIT),)

# wasm-decoder: (re)build the ac3go WebAssembly decoder (browser AC-3/E-AC-3
# path) from the pinned module (go.mod `tool` directive) into the web app's
# public dir. The .wasm + Go runtime shim are build artifacts (git-ignored) - run
# this after pulling or after bumping the ac3go pin (go get -tool ...@version).
# In CI/Docker the `wasmbuild` stage does the same. Runs in the evey-server
# container (Go toolchain + module cache).
# Standard go, NOT TinyGo: TinyGo never runs finalizers, so its js bridge
# leaks every crossing value (+6.75 MB/segment measured 2026-07-28). Builds
# from the sibling ../ac3go checkout; ship wasm_exec.js from the same
# compiler as the module.
wasm-decoder:
	docker run --rm -v $(PWD)/../ac3go:/src/ac3go -v $(PWD)/frontend/apps/web/public/wasm:/out -w /src/ac3go -e HOME=/tmp \
	  golang:1.26.6 sh -c '\
	  CGO_ENABLED=0 GOOS=js GOARCH=wasm go build -o /out/ac3go.wasm ./cmd/ac3go-wasm \
	  && cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" /out/wasm_exec.js'
	@echo "✅ ac3go.wasm (standard go) + wasm_exec.js (go shim) rebuilt in frontend/apps/web/public/wasm/"

test: test-image
	docker run --rm \
	  -v $(PWD):/app \
	  -v /var/run/docker.sock:/var/run/docker.sock \
	  -v evey-go-mod:/go/pkg/mod \
	  -v evey-go-build:/root/.cache/go-build \
	  --add-host=host.docker.internal:host-gateway \
	  -w /app \
	  evey-test \
	  sh -c 'go test $$(go list ./... | grep -v /gen/) -race -count=1 -timeout=600s'

# Copy-rung end-to-end contract test against the BUNDLED ffmpeg (runs inside
# the evey-server dev container - never in CI, where a distro ffmpeg would
# diverge from prod behavior). Run before shipping any copy-pipeline or
# ffmpeg-image change: real producer + real ffmpeg/ffprobe on a synthetic
# B-frame MKV, asserting served segments match the keyframe plan (timeline
# AND audio content correlation).
transcode-itest:
	docker exec -e GOTOOLCHAIN=auto evey-server sh -c 'cd /app && EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/ffmpeg/ ./internal/adapters/driven/mkvgo/ -run Integration -count=1 -v -timeout=300s'

test-image:
	docker build -t evey-test -f docker/Dockerfile.test .

test-shell: test-image
	docker run --rm -it \
	  -v $(PWD):/app \
	  -v /var/run/docker.sock:/var/run/docker.sock \
	  -v evey-go-mod:/go/pkg/mod \
	  -v evey-go-build:/root/.cache/go-build \
	  --add-host=host.docker.internal:host-gateway \
	  -w /app \
	  evey-test \
	  sh

lint:
	golangci-lint run ./...

# Curated gosec security sweep - uses a SEPARATE config (.golangci.gosec.yml)
# so `make lint` stays noise-free. Full sweep on demand (~21 findings, mostly
# already-triaged FPs). CI runs the same profile with only-new-issues as a
# regression ratchet. See gosec_eval memory for the triage.
sec-scan:
	golangci-lint run -c .golangci.gosec.yml ./...

# ── Mutation testing ──────────────────────────────────────────────────────────
# mutation-test: run gremlins over ONE package and report which mutants survive.
#
# A surviving mutant is a change to the production code that every test still
# passes through - i.e. a line the suite does not actually verify. That is the
# question "how many tests do we have?" cannot answer, and it found real gaps:
# RetryAfter's arithmetic had 6 survivors because one assertion used a 1..3s
# window where the answer is exactly 2s.
#
# Why a container and a clean checkout, not the host:
#   - gremlins 0.5.0 walks the module root and has no --exclude. This repo's root
#     also holds the bind-mounted Docker volumes (postgres-data/, caddy-data/,
#     evey-data/), owned by root with restrictive modes, so the walk died on
#     permission denied before running a single mutant. They are all gitignored,
#     so `git archive` yields a tree where they simply do not exist - the problem
#     is removed rather than worked around.
#   - it recompiles and re-runs the package's tests once per mutant, which is the
#     workload that wedges WSL. In a container it gets explicit cpu/memory caps.
#   - it runs as the invoking user (not root), so the checkout stays writable
#     without widening any permissions.
#
# Usage:  make mutation-test PKG=./internal/pkg/ratelimit/
#         make mutation-test PKG=./internal/app/transcoding/ DRY=1
# Scope it to ONE package: a whole-repo run is hours, and the report is only
# actionable per package anyway.
PKG ?= ./internal/pkg/ratelimit/
MUTATION_SRC := $(CURDIR)/.mutation-src
MUTATION_REPORT := $(CURDIR)/.mutation-report.txt
mutation-test:
	@command -v git >/dev/null || { echo "git required"; exit 1; }
	@docker build -q -t evey-mutation -f docker/Dockerfile.mutation . >/dev/null
	@rm -rf $(MUTATION_SRC) && mkdir -p $(MUTATION_SRC)
	@git archive HEAD | tar -x -C $(MUTATION_SRC)
	@# `gen/` is gitignored, so git archive drops it - and WITHOUT it nothing that
	@# imports the generated protobuf code compiles. gremlins buckets a test
	@# command that fails to BUILD as KILLED, so the run then reports a perfect
	@# 100% efficacy with zero survivors instead of erroring: a silently wrong
	@# result. Proven on ./internal/adapters/driving/http (2026-07-30): 1647
	@# "killed" in 7.6s, and a mutant it called KILLED survives a real test run.
	@# Copy the generated tree in rather than trusting the archive.
	@test -d gen || { echo "gen/ missing - run 'make proto-gen' first"; exit 1; }
	@cp -r gen $(MUTATION_SRC)/gen
	@# Guard against the same class of silent failure from any other cause: the
	@# package's own tests must pass in the clean checkout BEFORE mutating. A
	@# red baseline makes every mutant look killed.
	@echo "→ baseline check on $(PKG)"
	@docker run --rm --cpus 4 --memory 6g \
	  --user "$$(id -u):$$(id -g)" \
	  -e GOPATH=/tmp/go -e GOCACHE=/tmp/gocache -e HOME=/tmp \
	  -v $(MUTATION_SRC):/src -w /src \
	  evey-mutation go test $(PKG) -count=1 >/dev/null || \
	  { echo "BASELINE RED on $(PKG) - a mutation run would report a fake 100%"; rm -rf $(MUTATION_SRC); exit 1; }
	@echo "→ gremlins on $(PKG) (clean checkout, containerised, non-root)"
	@docker run --rm --cpus 4 --memory 6g \
	  --user "$$(id -u):$$(id -g)" \
	  -e GOPATH=/tmp/go -e GOCACHE=/tmp/gocache -e HOME=/tmp \
	  -v $(MUTATION_SRC):/src -w /src \
	  evey-mutation gremlins unleash $(if $(DRY),--dry-run,) $(PKG) \
	  | tee $(MUTATION_REPORT)
	@# PLAUSIBILITY GATE. A green baseline is not enough: on
	@# ./internal/adapters/driving/http gremlins reports "Killed 1647, Lived 0,
	@# efficacy 100%" in 7 SECONDS with a passing baseline - and a mutant it
	@# calls KILLED (webhook_handler.go:69, `encErr != nil` -> `== nil`)
	@# provably survives a real test run. Root cause unidentified in gremlins
	@# 0.5.0; what matters is that the lie is loud instead of silent.
	@#
	@# Zero survivors in a package with real mutants has never once been true in
	@# this repo (app/transcoding, far better tested, has 335). Treat it as a
	@# broken run, not as a perfect score.
	@awk '/^Killed:/ { split($$0, f, /[:,]/); k=f[2]+0; l=f[4]+0; \
	  if (l == 0 && k > 50) { \
	    print ""; \
	    print "IMPLAUSIBLE RESULT: " k " killed, ZERO survivors."; \
	    print "gremlins buckets a test command that fails to RUN as KILLED, so this"; \
	    print "is almost certainly a broken run, not a perfect suite. Cross-check by"; \
	    print "hand-applying one reported-KILLED mutant and running the package tests"; \
	    print "before believing any of it."; \
	    exit 1 } }' $(MUTATION_REPORT) || { rm -rf $(MUTATION_SRC) $(MUTATION_REPORT); exit 1; }
	@rm -rf $(MUTATION_SRC) $(MUTATION_REPORT)

# ── Pré-CI locale ─────────────────────────────────────────────────────────────
# Mirror exact des étapes CI (.github/workflows/ci.yml) qu'on peut rouler en
# local. Objectif : attraper les rouges (vulns, compile, lint, race) AVANT
# `git push` au lieu de découvrir 5 min après que la CI distante a échoué.
#
#   make ci-fast    quick: vet + govulncheck + build + sec-scan (~30-60s)
#                   ⇒ avant chaque push (couvre 90% des reds CI).
#   make ci-local   full:  ci-fast + race tests (unit + postgres) + frontend
#                          typecheck (~5-10 min)
#                   ⇒ avant un push important / refacto large.
#
# Tout passe par le container `evey-server` (Go pinné + cache modules monté)
# pour bypasser les soucis TLS/permissions sur l'hôte WSL.

ci-fast: ci-fast-vet ci-fast-vuln ci-fast-build ci-fast-sec
	@echo "✅ ci-fast OK - vet + govulncheck + build + gosec ratchet verts."
	@echo "   Tests -race PAS exécutés (utilise 'make ci-local' avant un push important)."

ci-fast-vet:
	@echo "→ go vet (excl. /gen/)"
	@docker exec -w /app -e GOTOOLCHAIN=auto evey-server sh -c 'go vet $$(go list ./... | grep -v /gen/)'

ci-fast-vuln:
	@echo "→ govulncheck (./cmd/... ./internal/...)"
	@docker exec -w /app -e GOTOOLCHAIN=auto evey-server sh -c 'test -x /go/bin/govulncheck || go install golang.org/x/vuln/cmd/govulncheck@latest; /go/bin/govulncheck ./cmd/... ./internal/...'

ci-fast-build:
	@echo "→ go build ./..."
	@docker exec -w /app -e GOTOOLCHAIN=auto evey-server sh -c 'go build ./...'

# ci-fast-unit: the CI `correctness` unit step, same package selection and same
# -race. It is here because vet, build and lint all pass on a test that asserts
# the WRONG value: the capped-CRF headroom shipped green through this gate and
# broke CI, since the only transcode check that ran was the path-triggered
# copy-pipeline itest, a different test entirely. -race matters as much as the
# suite itself - it is what surfaced the reap test's fork-vs-exec race.
#
# Deliberately NOT -count=1 (CI uses it, we don't): Go keys its cache on package
# AND flags, so keeping it costs nothing in fidelity and makes an unchanged push
# ~1s instead of minutes. Postgres is excluded exactly as in CI - it runs
# without -race, via repo-itest-gate.
ci-fast-unit:
	@echo "→ go test (unit, -race, excl. postgres+gen)"
	@docker exec -w /app -e GOTOOLCHAIN=auto evey-server sh -c 'go test $$(go list ./... | grep -v "adapters/driven/postgres" | grep -v /gen/) -timeout=600s -race'

# ci-fast-frontend: mirror of the CI `frontend` job, which gates the docker
# build just like the backend jobs do - yet nothing frontend-side ran in
# pre-push before, so a broken typecheck was only ever found on GitHub. Runs in
# evey-web (the host's pnpm/turbo hit root-owned .turbo dirs). Turbo caches, so
# an unchanged push is ~1s; a cold frontend build is ~16s.
#
# The `test` step was missing until 2026-07-23: 269 test files and ~33k lines
# of frontend tests existed but no gate anywhere - not turbo, not CI, not this
# target - ever ran them, so they could have been red for weeks unnoticed. The
# root `vitest run` covers every project at once (web, mobile, tv, store, api)
# in ~40s, which is why it is a single step rather than a per-app fan-out.
ci-fast-frontend:
	@echo "→ frontend typecheck (evey-web)"
	@docker exec -w /app evey-web sh -c 'pnpm run typecheck'
	@echo "→ frontend lint (oxlint, warnings are errors)"
	@docker exec -w /app evey-web sh -c 'pnpm run lint'
	@echo "→ frontend unit tests (vitest, all projects)"
	@docker exec -w /app evey-web sh -c 'pnpm run test'
	@echo "→ frontend build (evey-web)"
	@docker exec -w /app evey-web sh -c 'pnpm run build'

ci-fast-sec:
	@# Mirror du ratchet CI : --new-from-rev=<base> ne flag QUE les findings
	@# introduits par les commits non encore poussés. Sans ce flag, les
	@# ~21 FPs legacy bloqueraient chaque pre-push.
	@# BASE est calculé sur l'hôte (la résolution origin/<branche> est fiable
	@# depuis l'hôte) puis injecté dans l'exec. Fallback : origin/master,
	@# sinon HEAD~1.
	@BASE=$$(git merge-base "origin/master" HEAD 2>/dev/null || git rev-parse "origin/$$(git rev-parse --abbrev-ref HEAD)" 2>/dev/null || git rev-parse HEAD~1); \
	echo "→ golangci-lint -c .golangci.gosec.yml (new-from $$BASE)"; \
	docker exec -w /app -e GOTOOLCHAIN=auto evey-server sh -c "(command -v golangci-lint >/dev/null || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2) && golangci-lint run -c .golangci.gosec.yml --new-from-rev=$$BASE ./..."

ci-local-lint:
	@BASE=$$(git merge-base "origin/master" HEAD 2>/dev/null || git rev-parse "origin/$$(git rev-parse --abbrev-ref HEAD)" 2>/dev/null || git rev-parse HEAD~1); \
	echo "→ golangci-lint (general, new-from $$BASE)"; \
	docker exec -w /app -e GOTOOLCHAIN=auto evey-server sh -c "(command -v golangci-lint >/dev/null || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2) && golangci-lint run --new-from-rev=$$BASE ./..."

ci-local: ci-fast ci-local-lint ci-local-test ci-local-frontend
	@echo "✅ ci-local OK - mirror CI complet vert (vet + vuln + build + sec + lint + race + frontend typecheck)."

# ci-local-test reuse la cible `test` existante (testcontainers + race full suite).
ci-local-test: test

ci-local-frontend:
	@echo "→ frontend typecheck (pnpm --filter web)"
	@cd frontend && npx pnpm --filter web run typecheck

# pre-push: mirror of the three CI jobs that gate the docker build - `quality`
# (vet + govulncheck + build + gosec ratchet + lint), `correctness` (unit -race,
# postgres via repo-itest-gate) and `frontend` - run by the git pre-push hook so
# a red pipeline never surprises you 10 minutes after the push. Everything here
# is cache-backed (Go per package+flags, turbo per task), so an unchanged push
# costs seconds; only what you actually touched is recomputed.
#
# tv-audit runs on the HOST, deliberately: its guards compare the TV sources
# against the i18n catalogue at the repo root, and the evey-web container only
# mounts `frontend` at /app. Inside the container those guards skip themselves
# (visibly, as skipped tests) rather than pass on half the evidence - so this
# host-side run is the one that actually enforces them.
pre-push: ci-fast-vet ci-fast-vuln ci-fast-build ci-fast-unit ci-fast-frontend ci-fast-sec ci-local-lint ci-arch ci-proto-breaking repo-itest-gate transcode-itest-gate tv-audit
	@echo "✅ pre-push OK - quality gate green (vet + govulncheck + build + unit -race + frontend + gosec + lint + arch guards + proto compat + repo suite + transcode contract + audit TV)."

# ci-proto-breaking: `buf breaking` against the published tip.
#
# This check lived ONLY in .github/workflows/ci.yml, and CI has been switched
# off since 2026-07-29 (Actions quota). A push triggers nothing, so from that
# day a breaking proto change - a removed field, a renumbered tag, a changed
# type - would have shipped unnoticed, and the clients that consume these protos
# are ALREADY DEPLOYED (a beta APK is on a tester's phone). A wire break there
# is silent on the server and total on the client.
#
# Compares against origin/<branch>: the question a pre-push gate must answer is
# "is what I am about to publish compatible with what is published". When the
# remote ref is absent the target FAILS rather than passing - a gate that
# silently no-ops is worse than no gate (same doctrine as transcode-itest-gate).
# `safe.directory` is needed because the container runs as root over a tree
# owned by the host user.
PROTO_BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)
ci-proto-breaking:
	@echo "→ buf breaking (proto wire compat vs origin/$(PROTO_BRANCH))"
	@git rev-parse --verify --quiet "origin/$(PROTO_BRANCH)" >/dev/null || { \
	  echo "❌ origin/$(PROTO_BRANCH) is unknown locally - run 'git fetch origin' so this gate has something to compare against"; \
	  exit 1; }
	@docker exec -w /app evey-server sh -c '\
	  git config --global --add safe.directory /app >/dev/null 2>&1; \
	  git config --global --add safe.directory /app/.git >/dev/null 2>&1; \
	  buf breaking --against ".git#ref=origin/$(PROTO_BRANCH)"'

# ci-arch: the static architecture guards (TxUnit bypass, etc.) that need NO DB -
# they only regex the source tree, so they run in milliseconds. Their sibling
# postgres_test package boots a testcontainers Postgres (~130s) which is why the
# full suite is CI-only; these guards live in .../postgres/txguard so pre-push
# catches a TxUnit bypass locally instead of on CI.
ci-arch:
	@echo "→ arch guards (txguard + contractguard: no DB, sub-second)"
	@docker exec -w /app -e GOTOOLCHAIN=auto evey-server sh -c 'go test ./internal/adapters/driven/postgres/txguard/... ./internal/contractguard/...'

# transcode-itest-gate: run the copy-pipeline contract test (make transcode-itest)
# whenever the outgoing changes touch the transcoding stack - the bug class it
# catches (plan↔ffmpeg contract drift) is invisible to every host-side check in
# this gate, and it shipped audible desyncs three times in one day. Non-transcode
# pushes skip it (no 20s tax); transcode pushes REQUIRE the dev container (an
# explicit failure, never a silent skip - a gate that silently no-ops is worse
# than no gate). Force it on any push with TRANSCODE_ITEST=1.
#
# go.mod and go.sum are IN the list, and that is not belt-and-braces: the
# direct-play packager is a LIBRARY (mkvgo). Bumping it rewrites how every
# segment is cut and muxed without touching a single file of ours - the exact
# change this gate exists to catch, and the one it would wave through if it
# only watched our own paths.
# repo-itest-gate: run the REAL Postgres repository suite (testcontainers) when
# the outgoing changes touch domain invariants or the postgres adapter. Those
# two move together: a `Validate()` added to an aggregate is enforced by every
# repo `Create`, so a fixture that was legal yesterday starts failing - a class
# of break NO host-side check in this gate can see (it compiles, it vets, it
# lints; it only fails against a live database). Shipped exactly that way once:
# the ShareLink invariant landed green locally and broke CI on a DeleteExpired
# fixture whose link expired before it was created.
#
# Costs ~2-3 min (testcontainers boot + suite) and only on the pushes that can
# trigger it. Force it on any push with REPO_ITEST=1; skip with REPO_ITEST=0.
REPO_PATHS := internal/domain internal/adapters/driven/postgres

repo-itest-gate:
	@if [ "$(REPO_ITEST)" = "0" ]; then \
	  echo "→ repo suite skipped (REPO_ITEST=0)"; exit 0; \
	fi; \
	base=$$(git merge-base @{u} HEAD 2>/dev/null || git rev-parse 'HEAD~10' 2>/dev/null || git rev-parse HEAD); \
	if [ "$(REPO_ITEST)" = "1" ] || ! git diff --quiet $$base -- $(REPO_PATHS) 2>/dev/null; then \
	  echo "→ domain/postgres touched - running the real-Postgres repository suite"; \
	  $(MAKE) --no-print-directory repo-itest; \
	else \
	  echo "→ repo suite skipped (no domain/postgres change in this push)"; \
	fi

# repo-itest: the postgres package against a REAL Postgres (testcontainers).
# Runs in the test image because it needs the docker socket - the dev container
# has none. Same invocation as the CI `correctness` job, minus -race.
repo-itest: test-image
	docker run --rm \
	  -v $(PWD):/app \
	  -v /var/run/docker.sock:/var/run/docker.sock \
	  -v evey-go-mod:/go/pkg/mod \
	  -v evey-go-build:/root/.cache/go-build \
	  --add-host=host.docker.internal:host-gateway \
	  -w /app \
	  evey-test \
	  sh -c 'go test ./internal/adapters/driven/postgres/... -count=1 -timeout=600s'

# boot-itest: the cmd/evey composition-root smoke test (build-tagged `boottest`,
# so it never runs in pre-push). Spins a real Postgres via testcontainers, wires
# the FULL app (buildServices) and asserts every critical service is non-nil -
# the net that catches a decomposition dropping a service. Run before touching
# wire.go / buildAppServices. Needs the Docker socket (testcontainers).
boot-itest: test-image
	docker run --rm \
	  -v $(PWD):/app \
	  -v /var/run/docker.sock:/var/run/docker.sock \
	  -v evey-go-mod:/go/pkg/mod \
	  -v evey-go-build:/root/.cache/go-build \
	  --add-host=host.docker.internal:host-gateway \
	  -w /app \
	  evey-test \
	  sh -c 'go test -tags boottest -run TestBuildServices_Boot ./cmd/evey/ -count=1 -v -timeout=300s'

TRANSCODE_PATHS := internal/app/transcoding internal/domain/transcoding \
  internal/adapters/driven/ffmpeg \
  internal/adapters/driven/mkvgo \
  internal/adapters/driving/http/transcode_handler_vod.go \
  internal/adapters/driving/http/transcode_audio_handler.go \
  internal/adapters/driving/http/transcode_ondemand.go \
  go.mod go.sum \
  docker/Dockerfile docker/Dockerfile.dev

transcode-itest-gate:
	@base=$$(git merge-base @{u} HEAD 2>/dev/null || git rev-parse 'HEAD~10' 2>/dev/null || git rev-parse HEAD); \
	if [ "$(TRANSCODE_ITEST)" = "1" ] || ! git diff --quiet $$base -- $(TRANSCODE_PATHS) 2>/dev/null; then \
	  echo "→ transcode stack touched - running the copy-pipeline contract test (make transcode-itest)"; \
	  if ! docker inspect -f '{{.State.Running}}' evey-server 2>/dev/null | grep -q true; then \
	    echo "✗ evey-server container is not running - the transcode contract test CANNOT run."; \
	    echo "  Start the dev stack (docker compose up -d) and re-push. (No silent skip: this"; \
	    echo "  test is the only gate that sees plan↔ffmpeg contract regressions.)"; \
	    exit 1; \
	  fi; \
	  $(MAKE) --no-print-directory transcode-itest; \
	else \
	  echo "→ no transcode-stack changes since $$base - skipping transcode-itest"; \
	fi

# hooks: activate the committed git hooks (one-time, sets local core.hooksPath).
hooks:
	@git config core.hooksPath .githooks
	@echo "✓ git hooks active (core.hooksPath=.githooks) - push runs 'make pre-push' first."
	@echo "  Bypass one push intentionally: git push --no-verify"

clean:
	rm -rf bin/ gen/

# ── Database ───────────────────────────────────────────────────────────────────

# Migrations run automatically when evey-server boots (Air rebuilds → process
# restart → embedded RunMigrations). These targets exist for the operator
# path where you want to apply / roll back without bouncing the server (CI
# release pipeline, canary deploy, rollback drill).
#
# All three run *inside* the evey-server container: that's where Go, the
# source tree (/app), pg_dump, and the EVEY_DB_* env vars (sslmode=disable,
# host=postgres, EVEY_BACKUP_DIR=/data/backups) live. Running them on the
# WSL host would either crash on missing gcc / pg_dump or pick up the
# Postgres-incompatible "sslmode=prefer" default.
#
# evey-server must be up for these targets to work - start the stack first
# with `make dev` if it isn't.
migrate-up:
	@docker exec -w /app -e GOTOOLCHAIN=auto evey-server sh -c 'go run ./cmd/evey admin migrate up'

migrate-down:
	@docker exec -w /app -e GOTOOLCHAIN=auto evey-server sh -c 'go run ./cmd/evey admin migrate down'

migrate-version:
	@docker exec -w /app -e GOTOOLCHAIN=auto evey-server sh -c 'go run ./cmd/evey admin migrate version'

# ── Dev (uses docker-compose.yml + docker-compose.override.yml) ───────────────

# doctor: one-shot environment sanity check for contributors - each line tells
# you what is broken and how to fix it, instead of failing later mid-task.
doctor:
	@echo "── evey doctor ──────────────────────────────────────"
	@command -v go >/dev/null && echo "  ✓ go $$(go version | cut -d' ' -f3)" || echo "  ✗ go missing → https://go.dev/dl"
	@command -v docker >/dev/null && echo "  ✓ docker" || echo "  ✗ docker missing"
	@docker ps --format '{{.Names}}' 2>/dev/null | grep -q evey-server && echo "  ✓ evey-server container UP" || echo "  ✗ evey-server DOWN → make dev"
	@docker ps --format '{{.Names}}' 2>/dev/null | grep -q evey-postgres && echo "  ✓ evey-postgres UP" || echo "  ✗ evey-postgres DOWN → make dev"
	@docker ps --format '{{.Names}}' 2>/dev/null | grep -q evey-web && echo "  ✓ evey-web UP" || echo "  ✗ evey-web DOWN → make dev"
	@curl -sf http://localhost:9091/healthz >/dev/null 2>&1 && echo "  ✓ backend /healthz OK" || echo "  ✗ backend not answering on :9091 → docker logs evey-server"
	@docker exec evey-server sh -c 'command -v buf' >/dev/null 2>&1 && echo "  ✓ buf (in container)" || echo "  ⚠ buf not found in container (proto-gen unavailable)"
	@docker exec evey-server sh -c 'ffmpeg -version 2>/dev/null | head -1' 2>/dev/null | grep -q Jellyfin && echo "  ✓ jellyfin-ffmpeg (in container)" || echo "  ⚠ bundled ffmpeg missing → transcode-itest will fail"
	@grep -q evey.test /etc/hosts 2>/dev/null && echo "  ✓ evey.test in /etc/hosts" || echo "  ⚠ evey.test not in hosts → make setup"
	@echo "─────────────────────────────────────────────────────"

setup:
	@echo "→ Adding evey.test to WSL /etc/hosts..."
	@grep -q "evey.test" /etc/hosts || echo "127.0.0.1 evey.test" | sudo tee -a /etc/hosts

	@echo "→ Adding evey.test to Windows hosts..."
	@powershell.exe -Command "if (-not (Select-String -Path 'C:\Windows\System32\drivers\etc\hosts' -Pattern 'evey.test' -Quiet)) { Add-Content -Path 'C:\Windows\System32\drivers\etc\hosts' -Value '127.0.0.1 evey.test' }" 2>/dev/null \
		&& echo "  ✓ Windows hosts updated" \
		|| echo "  ⚠ Lance en admin dans PowerShell Windows: Add-Content C:\Windows\System32\drivers\etc\hosts '127.0.0.1 evey.test'"

	@echo ""
	@echo "✓ Setup done → lance 'make dev' (la génération buf/orval tourne au boot des containers dev)"

dev:
	$(DC) down --remove-orphans 2>/dev/null; \
	$(DC) up --build -d

dev-down:
	$(DC) down

dev-logs:
	$(DC) logs -f evey

# ── Infra only (run backend/frontend locally) ─────────────────────────────────

infra:
	$(DC) up postgres meilisearch -d

# ── Frontend ───────────────────────────────────────────────────────────────────

frontend-typecheck:
	$(DC) run --rm web pnpm --filter web typecheck

# ── TV ────────────────────────────────────────────────────────────────────────
#
# Une seule commande à retenir : `make tv-up`. Elle est idempotente et remonte
# tout ce qu'il faut (émulateur, adb, tunnels, Metro, bundle, app, connexion),
# quel que soit l'état de départ. Les autres cibles servent à observer.
#
# Le compte de dev vit dans scripts/tv/.env (ignoré par git), jamais ici :
#   EVEY_TV_USER=...
#   EVEY_TV_PASS=...

TV_SCRIPTS=scripts/tv

## tv-up: lance l'app TV sur l'émulateur (idempotent, < 30 s à chaud)
tv-up:
	@$(TV_SCRIPTS)/up.sh

## tv-up-fresh: idem mais sans la connexion automatique
tv-up-fresh:
	@$(TV_SCRIPTS)/up.sh --no-login

## tv-doctor: vérifie chaque prérequis et donne le remède exact
tv-doctor:
	@$(TV_SCRIPTS)/doctor.sh

## tv-state: état courant (device, Metro, écran, sha du bundle chargé)
tv-state:
	@$(TV_SCRIPTS)/state.sh

## tv-logs: journal de l'app seule. ARGS="--follow" ou ARGS="--since 5m motif"
tv-logs:
	@$(TV_SCRIPTS)/logs.sh $(ARGS)

## tv-shot: capture d'écran. NAME=<nom> facultatif
tv-shot:
	@$(TV_SCRIPTS)/shot.sh $(NAME)

## tv-key: touche télécommande. K=ok|up|down|left|right|back|play [N=3]
tv-key:
	@$(TV_SCRIPTS)/key.sh $(K) $(N)

## tv-login: connecte l'app avec le compte de dev
tv-login:
	@$(TV_SCRIPTS)/login.sh

## tv-reset: purge les données de l'app puis relance (cache de bundle corrompu)
tv-reset:
	@$(TV_SCRIPTS)/reset.sh

## tv-metro-restart: relance Metro avec le marqueur de build
tv-metro-restart:
	@$(TV_SCRIPTS)/metro-restart.sh

## tv-audit: règles TV (focus, a11y, résilience, parité tvOS, clés i18n)
tv-audit:
	@cd frontend && npx vitest run apps/tv/src/test/tvAudit.test.ts apps/tv/src/test/i18nKeys.test.ts

.PHONY: tv-up tv-up-fresh tv-doctor tv-state tv-logs tv-shot tv-key tv-login tv-reset tv-metro-restart tv-audit

# ── Mobile ────────────────────────────────────────────────────────────────────

ADB=$(HOME)/Applications/Android/Sdk/platform-tools/adb
MOBILE_DIR=frontend/apps/mobile
APK_PATH=$(MOBILE_DIR)/android/app/build/outputs/apk/release/app-release.apk

mobile-build:
	cd $(MOBILE_DIR)/android && ./gradlew assembleRelease --no-daemon

mobile-install: $(APK_PATH)
	$(ADB) uninstall com.evey.app 2>/dev/null || true
	$(ADB) install $(APK_PATH)
	$(ADB) reverse tcp:8080 tcp:8080
	$(ADB) shell am start -n com.evey.app/.MainActivity

mobile: mobile-build mobile-install

# Lance l'app Evey via Metro (container evey-mobile) + Expo Go sur l'émulateur
# courant. Pré-requis : `make dev` up + Expo Go installé sur l'émulateur.
mobile-go:
	adb reverse tcp:8081 tcp:8081
	adb reverse tcp:9091 tcp:9091
	adb shell am start -a android.intent.action.VIEW -d "exp://127.0.0.1:8081" host.exp.exponent
	@echo ""
	@echo "→ In the app, enter server URL: http://localhost:9091"
