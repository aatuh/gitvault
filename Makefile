BINARY=gitvault
SOPS_PATH?=sops
SOPS_RECIPIENT?=
SOPS_AGE_KEY_FILE?=

.PHONY: build test test-real-sops

build:
	mkdir -p bin
	go build -o bin/$(BINARY) ./cmd/gitvault
	cp README.md bin/README.md

test:
	go test ./...

test-real-sops:
	@set -e; \
	SOPS_PATH="$(SOPS_PATH)"; \
	KEY_FILE="$(SOPS_AGE_KEY_FILE)"; \
	if [ -z "$$KEY_FILE" ] && [ -n "$$SOPS_AGE_KEY_FILE" ]; then \
		KEY_FILE="$$SOPS_AGE_KEY_FILE"; \
	fi; \
	if [ -z "$$KEY_FILE" ]; then \
		DEFAULT_KEY="$$HOME/.config/sops/age/keys.txt"; \
		if [ -f "$$DEFAULT_KEY" ]; then \
			KEY_FILE="$$DEFAULT_KEY"; \
		else \
			TMP_DIR="$$(mktemp -d /tmp/gitvault-age-XXXXXX)"; \
			KEY_FILE="$$TMP_DIR/keys.txt"; \
		fi; \
	fi; \
	if [ ! -f "$$KEY_FILE" ]; then \
		if ! command -v age-keygen >/dev/null 2>&1; then \
			echo "age-keygen not found; install age or set SOPS_AGE_KEY_FILE."; \
			exit 1; \
		fi; \
		mkdir -p "$$(dirname "$$KEY_FILE")"; \
		age-keygen -o "$$KEY_FILE" >/dev/null; \
	fi; \
	if [ -n "$$TMP_DIR" ]; then \
		trap 'rm -rf "$$TMP_DIR"' EXIT; \
	fi; \
	if ! command -v "$$SOPS_PATH" >/dev/null 2>&1; then \
		echo "sops not found at $$SOPS_PATH"; \
		exit 1; \
	fi; \
	if [ -z "$(SOPS_RECIPIENT)" ]; then \
		if ! command -v age-keygen >/dev/null 2>&1; then \
			echo "age-keygen not found; set SOPS_RECIPIENT to skip deriving it."; \
			exit 1; \
		fi; \
		RECIPIENT="$$(age-keygen -y "$$KEY_FILE")"; \
	else \
		RECIPIENT="$(SOPS_RECIPIENT)"; \
	fi; \
	go test ./integration -real-sops -sops-path "$$SOPS_PATH" -sops-recipient "$$RECIPIENT" -sops-age-key-file "$$KEY_FILE"
