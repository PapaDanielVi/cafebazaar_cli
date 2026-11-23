# Contributing to `bazaar`

Thanks for helping improve `bazaar`. Bug reports, documentation fixes, new Gradle DSL test cases, and Nasher Publisher API schema updates are all welcome.

## Development Setup

Requirements:

- Go 1.24 or newer
- Git

Clone the repository and run the full verification suite:

```bash
git clone https://github.com/PapaDanielVi/cafebazaar_cli.git
cd cafebazaar_cli
make test
make vet
make build
```

## Coding Standards

Please keep pull requests aligned with the repository's Go conventions:

- Define constants for any literal value used more than once.
- Pass `context.Context` as the first parameter on all custom functions and methods.
- Write all `.go` comments as full sentences ending with a period.
- Use external test packages (`package <name>_test`) and call `t.Parallel()` in table-driven tests where safe.
- Use `errors.New` for static errors and `fmt.Errorf` only when formatting is needed.
- Check and handle every returned error.
- Keep JSON and YAML output in raw logical UTF-8 without HTML escaping so Persian (Farsi) text stays intact for scripts and CI jobs.

## Submitting Changes

1. Open an issue first for larger changes so we can agree on CLI flags or behavior.
2. Create a focused branch from `main`.
3. Add or update unit tests covering your changes.
4. Run `go vet ./...` and `go test -race -count=1 ./...` before opening a pull request.
