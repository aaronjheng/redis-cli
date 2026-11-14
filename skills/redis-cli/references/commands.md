# Commands

Starting point for picking and shaping Redis commands with this CLI. The server, not this client,
validates command names and arguments: unfamiliar commands are passed through verbatim, so the
Redis command reference is the source of truth.

## Invocation shape

```text
redis [flags] <COMMAND> [arg ...]
```

- Command and arguments are separate shell words: `redis SET key value`, `redis CONFIG GET maxmemory`,
  `redis ACL SETUSER app on '>secret'`. Quoting is the shell's job, not the CLI's; unquoted `>`,
  `*`, `[` and similar characters are expanded or redirected by the shell before the CLI sees them.
- Command names, subcommand keywords and option values are case-insensitive only where the
  server says so; key names, channel names and hash fields are always case-sensitive.
- Flags come before the command. `--` ends flag parsing so arguments that start with `-` are passed
  through: `redis -- SET --dash-value ok`.
- `-h` is the host flag, not help. Use `--help`.
- With no arguments the CLI starts a REPL (interactive only: line editing, completion from the
  bundled `commands.json`, `help <COMMAND>`, `exit`). Agents should always pass a command.
- `redis COMMAND INFO GET` / `redis COMMAND COUNT` ask the server what it supports; the interactive
  `help get` command is local documentation and unavailable in one-shot mode.

## Safe, read-only first

```bash
redis PING                                  # PONG, reachability + auth
redis INFO                                  # everything (always raw)
redis INFO SERVER                           # run_id, redis_version, tcp_port, uptime
redis INFO KEYSPACE                         # dbN:keys=…,expires=…,avg_ttl=…
redis DBSIZE                                # number of keys in the selected database
redis TYPE mykey                            # string/list/set/zset/hash/stream/none
redis TTL mykey                             # seconds, -1 = no expiry, -2 = missing
redis EXISTS key1 key2                      # count of existing keys
redis SCAN 0 MATCH 'prefix:*' COUNT 500     # cursor + keys, never blocks for long
redis MEMORY USAGE mykey                    # bytes, per-key estimate
redis OBJECT ENCODING mykey                 # listpack/quicklist/intset/hashtable/…
redis GET key                               # string value or nil
redis MGET key1 key2 key3                   # one line per value, in request order
redis STRLEN key
redis HGETALL myhash                        # key/value lines alternately (see output.md)
redis HLEN myhash
redis LRANGE mylist 0 -1                    # one element per line
redis SMEMBERS myset
redis SCARD myset
redis ZRANGE myzset 0 -1 WITHSCORES
redis XRANGE mystream - + COUNT 10
redis XINFO STREAM mystream
redis CLIENT LIST                           # other connections; treat as sensitive
redis CONFIG GET maxmemory                  # parameter name and value, one per line
redis SLOWLOG LEN
redis LATENCY HISTORY command
```

`SCAN` over `KEYS`: `KEYS pattern` walks the whole keyspace in one blocking operation and is
unsuitable for large or production databases. Use the `SCAN` loop in
[output.md](output.md#scan-without-keys).

## Writes and administrative commands

These change state. Confirm the target and the intent with the user first, and prefer the smallest
possible scope.

| Command | Risk |
|---|---|
| `SET`, `MSET`, `HSET`, `LPUSH`, `SADD`, `ZADD`, `XADD` | overwrite/append data |
| `DEL`, `UNLINK`, `EXPIRE`, `PERSIST`, `RENAME`, `RESTORE`, `SWAPDB` | data loss or unexpected key movement |
| `EVAL` / `--eval` scripts that call write commands | can modify many keys atomically |
| `CONFIG SET`, `CONFIG REWRITE`, `CONFIG RESETSTAT` | changes server behaviour at runtime |
| `FLUSHDB`, `FLUSHALL` | deletes an entire database / all databases |
| `SCRIPT FLUSH`, `FUNCTION FLUSH` | removes loaded code; other clients break |
| `CLIENT KILL`, `CLIENT NO-EVICT` | disconnects clients / changes eviction behaviour |
| `CLUSTER RESET`, `CLUSTER SETSLOT`, `CLUSTER FAILOVER` | can damage cluster availability |
| `REPLICAOF` / `SLAVEOF`, `MIGRATE`, `DEBUG`, `SHUTDOWN` | replication, migration, debug or shutdown |
| `ACL SETUSER`, `ACL DELUSER` | permission changes; `ACL LIST` exposes hashed secrets |

There is no `--dry-run`, read-only mode or confirmation prompt in this CLI. Anything you pass runs.

## Lua scripts

Use `--eval` for one-off scripts; it reads a file and infers `numkeys` from a bare `,` separator:

```bash
redis --eval check.lua key1 key2 , arg1 arg2
```

- Everything before `,` is a key (`KEYS`), everything after is an argument (`ARGV`); no `,` means
  all arguments are keys.
- Because of the separator handling, an empty string is appended to the end of `ARGV` when a comma
  is used. Read `ARGV[1]`… explicitly instead of trusting `#ARGV`.
- Scripts run atomically and block the server while they execute; keep them short and avoid loops
  over large keyspaces. Use shell-side `SCAN` loops for that.
- Prefer script results that are one string (for example `cjson.encode`) over tables, so raw output
  stays parseable. See [output.md](output.md#structured-output-json-via-lua).
- A script that errors exits `1` with the server's message. The server caches scripts by SHA; this
  CLI loads it transparently, so `NOSCRIPT` should not reach you.

One-liners are possible through `EVAL` itself, where the key count is explicit:

```bash
redis EVAL "return redis.call('DBSIZE')" 0
```

Quoting multi-line Lua in a shell is fragile; write a file and use `--eval` instead.

## Not available in this CLI

These official `redis-cli` conveniences do not exist here. Do not call them and do not pretend
they work: use the generic command, a shell loop or a script instead.

| Missing | Workaround |
|---|---|
| `--bigkeys`, `--memkeys`, `--hotkeys`, `--lru-test`, `--keystats` | `SCAN` + `MEMORY USAGE` / `OBJECT FREQ`, or sampling in a script |
| `--scan`, `--pattern`, `--count` convenience mode | `SCAN` loop in [output.md](output.md#scan-without-keys) |
| `--pipe` bulk loading, `--rdb`, `--replica` stream transfer | `EVAL`/`--eval` for batches, shell loops, or `BGSAVE` server-side |
| `--cluster create/add-node/reshard/…` | not implemented; drive `CLUSTER` commands directly or use the official `redis-cli --cluster` |
| `-x`/`-X` stdin payload, `-d`/`-D` delimiters, `--quoted-input` | build the arguments in the shell; stdin is not consumed |
| `-i` delay, repeat loops, `--stat`, `--latency*`, `--intrinsic-latency` | shell loops around single commands (`INFO`/`PING`) |
| `--json`, `--quoted-json`, `--csv`, `-2`/`-3`, `--show-pushes` | RESP2 only; return JSON from Lua via [output.md](output.md#structured-output-json-via-lua) |
| `-e` opt-in exit codes | not needed: this CLI already exits `1` on command errors |
| `-t` connect timeout, `--name` client name, `--askpass`, `--pass` | OS default timeout; `CLIENT SETNAME`; credentials via `-a`, URI or profile |
| `--no-raw` forced formatting | no counterpart: raw is used whenever stdout is not a TTY |
| `--no-auth-warning`, `REDISCLI_AUTH`, `--ldb`, `--ldb-sync-mode` | credentials come from flags/URI/profile; no Lua debugger integration |
| `-s` Unix socket, `-r` repeat count | here `-s` is TLS SNI and `-r` is the ACL user (see [SKILL.md](../SKILL.md)) |

## Exit behaviour to rely on

- `0` when the command produced a reply, including an empty array (which prints nothing).
- `1` for server errors, connection failures and argument problems; the reason is on stderr
  prefixed with `Error:`. See [output.md](output.md#errors-and-exit-codes).
