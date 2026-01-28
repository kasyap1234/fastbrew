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