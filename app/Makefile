# The desktop application. It is a separate Go module (the server and the agent must build without a web view).
VERSION ?= dev
# macOS: the framework needs 12 or newer; without it the linker warns about every object file
export MACOSX_DEPLOYMENT_TARGET ?= 12.0

.PHONY: frontend test build run headless clean

frontend:
	cd frontend && npm ci && npm run build && touch dist/.gitkeep

test: frontend
	cd frontend && npm run check && node --test src/lib/*.test.mjs
	go vet ./... && go test ./internal/...

build: frontend
	mkdir -p bin && go build -ldflags "-X main.version=$(VERSION)" -o bin/linager-app .

# the window of the application (a connection to a server is asked for on the first start)
run: build
	./bin/linager-app

# the interface on a local address, without a window: open the printed address in a browser (development, tests)
headless: build
	./bin/linager-app -headless

clean:
	rm -rf bin frontend/dist/* && touch frontend/dist/.gitkeep
