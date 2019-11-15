# Go parameters
BINARY=server-grpc

all: build

PLATFORMS := linux darwin
os = $(word 1, $@)

.PHONY: $(PLATFORMS)
$(PLATFORMS):
	GOOS=$(os) GOARCH=amd64 go build -o $(BINARY)-$(os)-amd64
.PHONY: build
build: linux darwin
