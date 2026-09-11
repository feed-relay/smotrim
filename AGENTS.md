# Feed Relay Provider for Smotrim

## Build and Test Commands
- Generate mocks: `make generate`
- Run all tests (race + coverage): `make test`
- Run tests with extended timeout: `make race`
- Lint code: `make lint`
- Fast pre-commit check: `make check` (fmt-check, vet, lint)
- Format code: `make fmt`

## Code Style & Patterns
- **Error Handling**: Always wrap errors using `fmt.Errorf("...: %w", err)` to provide context while preserving the original error for callers.
- **Mocking**: Use `moq` for interface mocks. Mocks are generated via `go generate ./...` and stored in `mocks/` directories.
- **Architecture**: 
    - Follow the pipeline flow: `Provider` -> `Client` -> `Adapter` -> `FileSizer` -> `XmlFileWriter`.
    - Keep the `Adapter` focused on mapping domain models to RSS formats.
    - Ensure the `XmlFileWriter` is the final stage that persists the output.
- **Linting**: Adhere to `golangci-lint` configurations defined in `.golangci.yml`.

## Important Workflow Notes 
- Always run tests and linter before committing
- For linter use `make lint`
- Run tests and linter after making significant changes to verify functionality
- Go version: 1.26+
- Don't add "Generated with Claude Code" or "Co-Authored-By: Claude" to commit messages or PRs
- After important functionality added, update README.md accordingly
- Do not add comments that describe changes, progress, or historical modifications. Avoid comments like "new function," "added test," "now we changed this," or "previously used X, now using Y." Comments should only describe the current state and purpose of the code, not its history or evolution.