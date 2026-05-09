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

## Git Workflow

### Critical Rules

- **NEVER commit or push changes unless the user EXPLICITLY asks you to.** Even if the user says "commit", do NOT also push unless they say "push". Do NOT assume the user wants to commit after making changes. Always wait for explicit instruction.

- Never run `git commit`, `git push`, or other git mutations unless explicitly instructed
- If explicitly instructed to commit or push, execute directly without extra confirmation
- Commit message rules:
  - One sentence only
  - No Conventional Commit prefixes
  - Capitalize the first letter
  - Example: "Replace kingpin with cobra"
