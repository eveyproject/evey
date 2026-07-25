.PHONY: proto-gen plugin-sdk plugin-zips build test test-image test-shell lint sec-scan clean \
	ci-fast ci-fast-vet ci-fast-vuln ci-fast-build ci-fast-sec \
	ci-local ci-local-lint ci-local-test ci-local-frontend ci-arch pre-push hooks \
	migrate-up migrate-down migrate-version setup dev dev-down dev-logs \
	infra frontend-typecheck \
	mobile-build mobile-install mobile mobile-go

BINARY=evey
DC=docker compose --env-file .env.dev

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
	go build -o bin/$(BINARY) ./cmd/evey

# wasm-decoder: (re)build the ac3go WebAssembly decoder (browser AC-3/E-AC-3
# path) from the pinned module (go.mod `tool` directive) into the web app's
# public dir. The .wasm + Go runtime shim are build artifacts (git-ignored) - run
# this after pulling or after bumping the ac3go pin (go get -tool ...@version).
# In CI/Docker the `wasmbuild` stage does the same. Runs in the evey-server
# container (Go toolchain + module cache).
# TinyGo, not standard go: 391 KB vs 2.9 MB, ~5x faster browser compile -
# decisive for the Cast receiver which compiles this module on a TV's CPU
# (10s -> ~2s measured 2026-07-25). Requires ac3go's streaming entrypoint
# (>= v0.0.0-20260725211506): the old one accumulated PCM on the Go heap and
# TinyGo ballooned to 768 MB. Warning: the wasm_exec.js shim is TINYGO'S,
# incompatible with standard go's - always ship both together.
wasm-decoder:
	docker run --rm -v $(PWD):/src -w /src -e HOME=/tmp -e GOFLAGS=-mod=mod \
	  tinygo/tinygo:0.41.1 sh -c '\
	  tinygo build -target wasm -no-debug -opt=z -o frontend/apps/web/public/wasm/ac3go.wasm github.com/gravity-zero/ac3go/cmd/ac3go-wasm \
	  && cp /usr/local/tinygo/targets/wasm_exec.js frontend/apps/web/public/wasm/wasm_exec.js'
	@echo "✅ ac3go.wasm (TinyGo) + wasm_exec.js (shim TinyGo) rebuilt in frontend/apps/web/public/wasm/"

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
pre-push: ci-fast-vet ci-fast-vuln ci-fast-build ci-fast-unit ci-fast-frontend ci-fast-sec ci-local-lint ci-arch repo-itest-gate transcode-itest-gate
	@echo "✅ pre-push OK - quality gate green (vet + govulncheck + build + unit -race + frontend + gosec + lint + arch guards + repo suite + transcode contract)."

# ci-arch: the static architecture guards (TxUnit bypass, etc.) that need NO DB -
# they only regex the source tree, so they run in milliseconds. Their sibling
# postgres_test package boots a testcontainers Postgres (~130s) which is why the
# full suite is CI-only; these guards live in .../postgres/txguard so pre-push
# catches a TxUnit bypass locally instead of on CI.
ci-arch:
	@echo "→ arch guards (txguard: no DB, sub-second)"
	@docker exec -w /app -e GOTOOLCHAIN=auto evey-server sh -c 'go test ./internal/adapters/driven/postgres/txguard/...'

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
