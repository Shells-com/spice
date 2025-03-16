# CLAUDE.md - Repository Guidelines

## Build, Lint and Test Commands
- Build: `make` or `make all` (runs goimports and builds)
- Install deps: `make deps`
- Run all tests: `make test` or `go test -v ./...`
- Test single package: `go test -v github.com/Shells-com/spice/quic`
- Test single test: `go test -v github.com/Shells-com/spice/quic -run TestQuic`
- Format code: `$(GOPATH)/bin/goimports -w -l .`

## Code Style Guidelines
- **Error Handling**: Custom error types with descriptive prefixes; idiomatic `if err != nil { return err }`
- **Naming**: CamelCase for types/structs/interfaces; camelCase for variables/methods; ALL_CAPS for constants
- **Imports**: Standard library first, third-party second, project imports last; group with parentheses
- **Formatting**: Standard Go formatting with tabs for indentation; blank lines between methods
- **Types**: Descriptive type names with domain prefixes; pointer receivers for methods on complex types
- **Testing**: Use testify/assert for assertions with descriptive error messages