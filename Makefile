BINARY_NAME=fastbrew
VERSION=$(shell git describe --tags 2>/dev/null || echo "dev")

build:
	go build -ldflags "-X fastbrew/cmd.Version=$(VERSION)" -o ${BINARY_NAME} main.go

run:
	./${BINARY_NAME}

clean:
	go clean
	rm -f ${BINARY_NAME}
	rm -rf dist/

install:
	go install

package: build
	mkdir -p dist
	tar -czvf dist/fastbrew-linux-amd64.tar.gz ${BINARY_NAME} README.md

# Check goreleaser config without releasing
release-check:
	goreleaser check

# Build binaries locally using goreleaser (snapshot mode, no publish)
release-snapshot:
	goreleaser release --snapshot --clean

perf-bench: build
	go run ./benchmark -mode all -runs 7

perf-bench-cold: build
	go run ./benchmark -mode cold -runs 7

perf-bench-warm: build
	go run ./benchmark -mode warm -runs 7

perf-compare: build
	go run ./benchmark -mode all -runs 11

perf-profile:
	go test -run '^$$' -bench . -benchmem ./internal/brew ./internal/services
