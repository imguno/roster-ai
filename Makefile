.PHONY: build install clean test dry-run dashboard

BINARY   = roster
SRC_DIR  = roster
OUT_DIR  = $(SRC_DIR)

dashboard:
	cd $(SRC_DIR)/internal/web/dashboard && npm ci && npm run build

build: dashboard
	cd $(SRC_DIR) && go build -o $(BINARY) ./cmd/roster

install: build
	cp $(SRC_DIR)/$(BINARY) /usr/local/bin/$(BINARY)
	@echo "roster installed to /usr/local/bin/roster"

clean:
	rm -f $(SRC_DIR)/$(BINARY)

test:
	cd $(SRC_DIR) && go test ./...

dry-run:
	cd $(SRC_DIR) && go run ./cmd/roster dry-run .
