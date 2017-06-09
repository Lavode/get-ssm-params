
BIN = get-ssm-params
VERSION = $(shell git describe --tags | cut -dv -f2)
LDFLAGS := -X main.AppVersion=$(VERSION) -w

OS ?= linux
ARCH ?= amd64

all: $(BIN)

$(BIN): main.go
	go get -d
	env GOOS=$(OS) GOARCH=$(ARCH) go build -ldflags "$(LDFLAGS) -s" -o "$(BIN)"

clean:
	rm -f $(BIN)
