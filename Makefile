.PHONY: proto-gen plugin-sdk plugin-zips build test test-image test-shell lint sec-scan clean \
	migrate-up migrate-down setup dev dev-down dev-logs \
	infra frontend-typecheck \
	mobile-build mobile-install mobile mobile-go

BINARY=evey
DC=docker compose --env-file .env.dev

# ── Build ──────────────────────────────────────────────────────────────────────

proto-gen:
	buf generate

plugin-sdk:
	@echo "Generating plugin SDK stubs..."
	@# Python — generate inside container to match grpcio version
	docker compose exec evey sh -c 'mkdir -p /tmp/sdk_gen && python3 -m grpc_tools.protoc --proto_path=/app/proto --python_out=/tmp/sdk_gen --grpc_python_out=/tmp/sdk_gen /app/proto/plugin/v1/plugin_api.proto'
	docker cp $$(docker compose ps -q evey):/tmp/sdk_gen/plugin/v1/plugin_api_pb2.py sdk/python/
	docker cp $$(docker compose ps -q evey):/tmp/sdk_gen/plugin/v1/plugin_api_pb2_grpc.py sdk/python/
	sed -i 's/from plugin\.v1 import /import /g' sdk/python/plugin_api_pb2_grpc.py
	@# Node.js — proto-loader (runtime), just copy proto
	cp proto/plugin/v1/plugin_api.proto sdk/nodejs/
	@# PHP — needs protoc + grpc_php_plugin (see Dockerfile.plugins.example)
	cp proto/plugin/v1/plugin_api.proto sdk/php/
	@# Ruby — needs grpc-tools gem (see Dockerfile.plugins.example)
	cp proto/plugin/v1/plugin_api.proto sdk/ruby/
	@# Java/Kotlin — needs protoc-gen-grpc-java (see Dockerfile.plugins.example)
	mkdir -p sdk/java
	cp proto/plugin/v1/plugin_api.proto sdk/java/
	@# Swift — needs protoc-gen-swift + protoc-gen-grpc-swift
	mkdir -p sdk/swift
	cp proto/plugin/v1/plugin_api.proto sdk/swift/
	@# Dart — needs protoc-gen-dart
	mkdir -p sdk/dart
	cp proto/plugin/v1/plugin_api.proto sdk/dart/
	@echo "Done — sdk/ updated"

plugin-zips:
	@./scripts/build-test-plugins.sh

build:
	go build -o bin/$(BINARY) ./cmd/evey

test: test-image
	docker run --rm \
	  -v $(PWD):/app \
	  -v /var/run/docker.sock:/var/run/docker.sock \
	  -v evey-go-mod:/go/pkg/mod \
	  -v evey-go-build:/root/.cache/go-build \
	  --add-host=host.docker.internal:host-gateway \
	  -w /app \
	  evey-test \
	  sh -c 'go test $$(go list ./... | grep -v /gen/) -race -count=1 -timeout=300s'

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

# Curated gosec security sweep — uses a SEPARATE config (.golangci.gosec.yml)
# so `make lint` stays noise-free. Full sweep on demand (~21 findings, mostly
# already-triaged FPs). CI runs the same profile with only-new-issues as a
# regression ratchet. See gosec_eval memory for the triage.
sec-scan:
	golangci-lint run -c .golangci.gosec.yml ./...

clean:
	rm -rf bin/ gen/

# ── Database ───────────────────────────────────────────────────────────────────

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

# ── Dev (uses docker-compose.yml + docker-compose.override.yml) ───────────────

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
