module github.com/albertocavalcante/bazelbump

go 1.25

toolchain go1.25.5

require (
	github.com/albertocavalcante/go-bcr v0.0.0-20260204170306-7a96b653fa0e
	github.com/albertocavalcante/starlark-syntax-go v0.0.0-20260204124100-a0f90b3b04d8
	github.com/gofrs/flock v0.13.0
	github.com/google/uuid v1.6.0
	github.com/qri-io/starlib v0.5.0
	go.starlark.net v0.0.0-20260102030733-3fee463870c9
	modernc.org/sqlite v1.44.3
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pkg/errors v0.8.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/sys v0.37.0 // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

// Starlark is added as a direct dependency for the recipe DSL system.
// See docs/design/starlark-recipes-spec.md for details.
