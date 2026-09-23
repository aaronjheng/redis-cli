# AGENTS.md

## Commands

```bash
# Build
go build ./cmd/redis/
go build -o redis-cli ./cmd/redis/

# Lint
just lint

# Lint with auto-fix
just lint-with-fix

# Update dependencies
just bump-deps

# Update commands.json from upstream
just update-commands

# Install
just install
```

## Code Quality

### Formatting

- Use `gofumpt` for formatting.
- Use `gci` for import ordering (standard, default, localmodule).

### Linting

- All lint errors must be fixed before committing.
- Do not enable deprecated linters.

## Commits & Pull Requests

- Commit message: no Conventional Commit prefixes, capitalize the first letter (e.g. "Replace kingpin with cobra").
