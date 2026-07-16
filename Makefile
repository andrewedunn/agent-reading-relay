.PHONY: build test tidy clean install

BIN_DIR := bin

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/reading-relay ./cmd/reading-relay
	go build -o $(BIN_DIR)/readerctl ./cmd/readerctl

test:
	go test ./...

tidy:
	go mod tidy

clean:
	rm -f $(BIN_DIR)/reading-relay $(BIN_DIR)/readerctl

# Deliberately not run automatically. Installing and enabling the service
# requires the user's explicit approval after the draft-only smoke test.
install: build
	sudo install -m 0755 $(BIN_DIR)/reading-relay /usr/local/bin/reading-relay
	sudo install -m 0755 $(BIN_DIR)/readerctl /usr/local/bin/readerctl
	sudo install -m 0644 reading-relay.service /etc/systemd/system/reading-relay.service
	sudo systemctl daemon-reload
