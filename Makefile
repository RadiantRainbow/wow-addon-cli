GO_SRC := $(shell find . -name *.go)
GO_PREREQS := $(GO_SRC) go.mod go.sum
GO_PREREQS_TEST := $(GO_PREREQS) .cache/test

bin/wow-addon-cli: $(GO_PREREQS_TEST)
	CGO_ENABLED=0 go build -o bin/wow-addon-cli .

bin/wow-addon-cli-linux-amd64: $(GO_PREREQS_TEST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ .

bin/wow-addon-cli-linux-arm64: $(GO_PREREQS_TEST)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $@ .

bin/wow-addon-cli-windows-amd64.exe: $(GO_PREREQS_TEST)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $@ .

bin/wow-addon-cli-windows-arm64.exe: $(GO_PREREQS_TEST)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -o $@ .

.cache/test: $(GO_PREREQS)
	mkdir -p .cache/ && CGO_ENABLED=0 go test ./... && touch $@


.PHONY: dist
dist: bin/wow-addon-cli-linux-amd64 bin/wow-addon-cli-linux-arm64 bin/wow-addon-cli-windows-amd64.exe bin/wow-addon-cli-windows-arm64.exe
	rm -rf dist/ && mkdir -p dist/ && cp $^ -t dist/ && (cd dist && sha256sum * > sha256sum.txt)
