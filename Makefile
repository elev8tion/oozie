.PHONY: run test build app install-app

run:
	go run ./cmd/app

test:
	go test ./...

build:
	go build ./cmd/app

# Build dist/oozie.app — a double-clickable Mac app.
app:
	sh scripts/make-app.sh

# Build and install oozie.app into /Applications.
install-app: app
	mkdir -p /Applications
	rm -rf /Applications/oozie.app ~/Applications/oozie.app
	ditto dist/oozie.app /Applications/oozie.app
	@echo "installed /Applications/oozie.app"
