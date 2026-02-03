module github.com/albertocavalcante/bazelbump

go 1.25

toolchain go1.25.5

// Zero direct dependencies by design.
// External code is vendored into third_party/ or data/.
// Dev tools use separate go.mod files in tools/.
