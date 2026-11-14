---
name: redis-cli
description: >-
  Runs Redis commands against a live server from a terminal or coding-agent session using the
  `redis` binary from aaronjheng/redis-cli, the Go alternative to the official redis-cli. Installs
  it with go install, connects through a Redis URI, a named YAML profile, TLS (rediss://), an SSH
  tunnel or a cluster seed, and executes commands and Lua scripts non-interactively with raw,
  machine-readable output. Use when the user asks to query, inspect or manage Redis data, keys,
  metrics or configuration (PING, INFO, DBSIZE, SCAN, GET, HGETALL, CLUSTER INFO), to test
  connectivity or debug a Redis dial, TLS, SSH tunnel, authentication, profile or cluster redirect
  error, or to script Redis from CI or an agent. Not for Redis client libraries in application
  code (go-redis, redis-py, ioredis, Lettuce, StackExchange.Redis), not for installing or tuning a
  Redis server, and not for features this CLI lacks such as --bigkeys, --memkeys, --pipe, --rdb,
  --replica or --cluster create.
license: Apache-2.0
metadata:
  version: "1"
  repository: https://github.com/aaronjheng/redis-cli
---

# Redis CLI (Go)

`redis` is a Go client for Redis and Redis Cluster built from this repository. It speaks RESP to
the server, resolves connection settings from flags or a YAML profile file, runs one command (or
one Lua script) and exits, which is what a coding agent or CI job needs. With no command arguments
it starts an interactive REPL instead — convenient for humans, a hang for an agent.

## Check which client you have

This skill covers the Go binary named `redis`, not the official `redis-cli`. Confirm which one you
have before running anything:

```bash
command -v redis redis-cli
redis -v        # this CLI:      redis version 1.0.0 (go1.27.3 linux/amd64)
                # official CLI: redis-cli 7.4.1
```

Only this CLI has `-P/--profile`, `-f/--config`, `--ssh`, `--ssh-identity-file`, `--ssh-mode` and
`--certb64`. Some short flags collide and change meaning:

| Flag | Go `redis` (this CLI) | Official `redis-cli` |
|---|---|---|
| `-s` | TLS SNI | Unix domain socket |
| `-r` | ACL user (same as `--user`) | repeat count |
| `-P` | profile name | no such option |
| `-f` | config file path | no such option |

`-h`, `-p`, `-n`, `-a`, `-u`, `-c`, `--tls`, `--raw` and `--eval` mean the same in both. Do not
copy special-mode flags (`--bigkeys`, `--scan`, `--pipe`, `--rdb`, `--replica`, `--stat`, `--json`,
`--csv`, `-x`, `-e`, `-i`) to this client; see
[references/commands.md](references/commands.md#not-available-in-this-cli).

If only `redis-cli` exists, do not translate commands from this skill blindly; run
`redis-cli --help` and use the matching official flags (or install this CLI as below). Never
invent flags that the installed binary does not list.

## Install

Go 1.27 or newer (the module declares `go 1.27.0`):

```bash
go install github.com/aaronjheng/redis-cli/cmd/redis@latest
redis -v
```

The binary is installed as `redis` into `$(go env GOPATH)/bin` (`~/go/bin` by default); add that
directory to `PATH` when `redis` is not found. Pinning to a commit is possible but unnecessary for
normal use; `@latest` is the supported install path.

## Route by task

| The user wants to… | Read |
|---|---|
| connect: URI, profile, TLS, SSH tunnel, cluster, config file, env vars | [references/connections.md](references/connections.md) |
| understand or parse what the CLI prints, handle errors and exit codes, get JSON | [references/output.md](references/output.md) |
| choose the right Redis command, run Lua, know what this CLI does not support | [references/commands.md](references/commands.md) |
| fix a failure (dial, auth, TLS/certificate, SSH, cluster redirect, profile) | [references/connections.md](references/connections.md) |
| know why output looks flattened, unquoted or missing labels | [references/output.md](references/output.md) |

## Run commands non-interactively

```bash
redis PING                                   # uses the default profile / 127.0.0.1:6379
redis INFO KEYSPACE                          # INFO is always printed raw
redis SCAN 0 MATCH 'session:*' COUNT 500     # SCAN cursor, then keys, one per line
redis GET 'user:42:name'
redis HGETALL 'user:42'
redis --eval script.lua key1 key2 , arg1 arg2
```

Rules for agents:

- **Always pass at least one argument.** `redis` with no command opens the REPL and waits on
  stdin, which looks like a hang in an agent step.
- The command and its arguments are separate shell words. There is no "command string" parser:
  `redis GET "my key"`, not `redis 'GET my key'`. Command names are case-insensitive.
- Flags go before the command. Use `--` to stop flag parsing when an argument itself starts with a
  dash: `redis -- SET --weird value`.
- Output is **raw automatically whenever stdout is not a TTY**, so piped, CI and agent runs need no
  extra flag; `--raw` forces raw even in a terminal. `INFO` is always raw. There is no flag to
  force the human-readable format (the official CLI's `--no-raw` has no counterpart here).
- Exit code is `0` on success and `1` on any failure. Server error replies and connection failures
  go to **stderr** with an `Error: …` prefix, so check the exit code rather than assuming stdout
  is the whole answer.
- `-h` means **host** here, never help. `--help` prints usage.
- Read-only discovery (`SCAN`, `TYPE`, `TTL`, `INFO`, `DBSIZE`) before mutating anything.

## What the output actually looks like

Raw output is a flat, line-oriented rendering of the RESP reply. That is ideal for one-line values
and shell pipelines, and lossy for structured replies:

- arrays print one element per line, with no element count, index or key labels;
- nested arrays are flattened further, so `HGETALL`, `CONFIG GET` and `XRANGE` print key and value
  lines alternately;
- bulk strings are unquoted, so a value that itself contains newlines spans several lines and
  cannot be told apart from separate elements;
- `nil` means a null reply; an empty line means an empty string;
- integers print as bare digits (no `(integer)`) and error text has no `(error)` marker.

In short: parse scalars from raw output, but do not parse pairs, nested replies or multi-line
values by splitting lines. For anything structural, have the server produce JSON from Lua and
return it as one string — see
[references/output.md](references/output.md#structured-output-json-via-lua). Do not invent a
`--json` flag; this CLI has none.

## Safety rules

- This CLI has **no read-only mode, dry run or confirmation prompt**. Whatever you pass is
  executed. Verify the target (`redis INFO SERVER`, `redis DBSIZE`, `redis CONFIG GET databases`)
  before any write, and ask the user for confirmation before destructive or irreversible
  commands: `FLUSHALL`, `FLUSHDB`, `DEL`/`UNLINK` in bulk, `KEYS` on a large database,
  `SHUTDOWN`, `CONFIG SET`, `CONFIG RESETSTAT`, `DEBUG`, `SCRIPT FLUSH`, `CLUSTER RESET`,
  `REPLICAOF`, `MIGRATE`, `SWAPDB`, and `EVAL` scripts that write.
- Prefer `SCAN` over `KEYS`. `SCAN` is incremental and returns after a bounded amount of work,
  instead of blocking the server for the whole keyspace like `KEYS`.
- `MONITOR` never exits on its own: it streams every command line until killed. Always wrap it
  in a timeout (for example `timeout 10 redis MONITOR`). Blocking commands with an infinite
  timeout (`BLPOP key 0`, `BRPOP key 0`, `XREAD BLOCK 0 …`, `WAIT 0 …`) also hang; give them a
  finite timeout or wrap the call in `timeout`. `SUBSCRIBE`/`PSUBSCRIBE` are not streamed by this
  CLI: they print the confirmation and exit.
- Keep credentials out of shell history, logs and `ps` output: put them in a profile file with
  restrictive permissions, or in a `rediss://` URI. There is no `REDISCLI_AUTH` environment
  variable; `-a` is visible in the process list. CA material can come from `REDIS_CACERT` or
  `REDIS_CERTB64`.
- Do not weaken TLS to "make it work": prefer `--cacert` for a private CA over `--insecure`, and
  say so when you use `--insecure`.
- `MONITOR` shows other clients' traffic and `INFO` can expose internal state. Treat captured
  output as sensitive.
- Cluster mode accepts database `0` only; `-n` with a non-zero database fails.

## Troubleshooting first moves

| Symptom | First move |
|---|---|
| `Error: dial: dial: dial URL: dial tcp …: connect: connection refused` | server or tunnel port is not listening; check host/port, firewall, SSH tunnel |
| `WRONGPASS` / `NOAUTH` / `ERR AUTH` | wrong password, wrong ACL user, or auth not configured; test with `redis PING` |
| `x509: certificate signed by unknown authority` | trust the CA with `--cacert` (PEM file) or `REDIS_CACERT`; `--insecure` only with explicit consent |
| `tls: first record does not look like a TLS handshake` or `TLS handshake timeout` | the endpoint is plaintext or the port is wrong; a `redis://` URI with `--tls` stays plaintext — use `rediss://` |
| `resolve profile: profile not found: <name>` | typo in `-P`, or `default_profile` points at a missing profile |
| `load config: read config: …` | an explicit `-f` file is missing or invalid YAML |
| `redis cluster supports database 0 only: got database N` | drop `-n`/`--db` or use `0` in cluster mode |
| `too many cluster redirects` | seed node cannot reach its cluster peers, or the topology is changing |
| SSH tunnel cannot authenticate | builtin mode is key-only; external mode can read `~/.ssh/config`, `ssh-agent` and prompts |
| `MONITOR` / blocking command, no timeout | `MONITOR` streams until killed; `BLPOP`/`BRPOP`/`XREAD BLOCK` with timeout `0` wait forever |
| No output at all, prompt never returns | no command arguments: the REPL is waiting on stdin |

Exact flags, config fields and precedence rules are in
[references/connections.md](references/connections.md).
