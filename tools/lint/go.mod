module github.com/albertocavalcante/bazelbump/tools/lint

go 1.25

toolchain go1.25.5

tool (
	github.com/golangci/golangci-lint/v2/cmd/golangci-lint
	golang.org/x/tools/cmd/goimports
	honnef.co/go/tools/cmd/staticcheck
)
