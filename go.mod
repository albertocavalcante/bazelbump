module github.com/albertocavalcante/bazelbump

go 1.25

toolchain go1.25.5

require (
	github.com/qri-io/starlib v0.5.0
	go.starlark.net v0.0.0-20260102030733-3fee463870c9
)

require (
	github.com/pkg/errors v0.8.1 // indirect
	golang.org/x/sys v0.0.0-20220715151400-c0bba94af5f8 // indirect
)

// Starlark is added as a direct dependency for the recipe DSL system.
// See docs/design/starlark-recipes-spec.md for details.
