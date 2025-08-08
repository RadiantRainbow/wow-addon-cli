GO_SRC := $(shell find . -name *.go)

bin/wow-addon-cli: $(GO_SRC) go.mod go.sum
	go build -o bin/wow-addon-cli ./cmd/wow-addon-cli
