# Code Linting

This project uses `golangci-lint` for code quality checks. The same linter configuration is used in both local development and CI/CD pipelines.

## Linter Configuration

- **Linter**: `golangci-lint` version `v2.1.0`
- **Configuration file**: `.golangci.yml`
- **GitHub Actions workflow**: `.github/workflows/lint.yml`

## Local Setup

### Install golangci-lint

The project includes a Makefile target to automatically download the correct version:

```bash
make golangci-lint
```

This will download `golangci-lint v2.1.0` to `bin/golangci-lint`.

### VS Code Integration

The project includes VS Code settings (`.vscode/settings.json`) that automatically:
- Use `golangci-lint` as the Go linter
- Run linting on save
- Format code with `goimports` on save
- Organize imports automatically

**Recommended VS Code extensions:**
- `golang.go` - Official Go extension
- `golangci-lint.golangci-lint` - golangci-lint extension

Install them via `.vscode/extensions.json` recommendations.

## Running the Linter

### Check for linting errors

```bash
make lint
```

### Auto-fix linting issues

```bash
make lint-fix
```

### Verify linter configuration

```bash
make lint-config
```

## Enabled Linters

The following linters are enabled in `.golangci.yml`:

- **copyloopvar** - Detects loop variables that are copied
- **dupl** - Tool for code clone detection
- **errcheck** - Checks for unchecked errors
- **ginkgolinter** - Enforces Ginkgo best practices
- **goconst** - Finds repeated strings that could be replaced by a constant
- **gocyclo** - Computes cyclomatic complexity
- **govet** - Reports suspicious constructs
- **ineffassign** - Detects ineffectual assignments
- **lll** - Reports long lines
- **misspell** - Finds commonly misspelled English words
- **nakedret** - Finds naked returns in functions greater than a specified length
- **prealloc** - Finds slice declarations that could potentially be pre-allocated
- **revive** - Fast, configurable, extensible linter for Go
- **staticcheck** - Static analysis for Go
- **unconvert** - Removes unnecessary type conversions
- **unparam** - Reports unused function parameters
- **unused** - Checks for unused constants, variables, functions and types

## Formatters

- **gofmt** - Standard Go formatter
- **goimports** - Updates Go import lines, adding missing ones and removing unreferenced ones

## CI/CD Integration

The GitHub Actions workflow (`.github/workflows/lint.yml`) automatically runs the linter on:
- Push to `main`, `master`, or `develop` branches
- Pull requests

**Important**: All code must pass linting checks before merging.

## Best Practices

1. **Run linting before committing**: Always run `make lint` before committing code
2. **Fix auto-fixable issues**: Use `make lint-fix` to automatically fix issues
3. **Check CI status**: Ensure GitHub Actions linting passes before merging
4. **IDE integration**: Use VS Code settings for automatic linting on save
5. **Review lint errors**: Understand and fix linting errors, don't just suppress them

## Troubleshooting

### Linter not found

If you see "golangci-lint: command not found", run:

```bash
make golangci-lint
```

### Configuration errors

Verify your configuration:

```bash
make lint-config
```

### Version mismatch

Ensure you're using the same version as CI (`v2.1.0`). The Makefile automatically downloads the correct version.

### Excluding files

Files in `api/`, `internal/`, `third_party`, `builtin`, and `examples` have some linter exclusions. See `.golangci.yml` for details.

