# Output and errors

What an agent or script actually receives from `redis`, and how to parse it without guessing.

## Two renderers

- **Raw** — used automatically when stdout is not a TTY (pipes, CI, agent tool calls), forced by
  `--raw`, and always used for `INFO`. One line per scalar reply element.
- **Human** — only when stdout is a TTY and `--raw` is not given: numbered lists, quoted bulk
  strings, `(integer)` and `(error)` prefixes. There is no flag to force this format.

Because raw is automatic off a TTY, agent output never contains `(integer)`, `1)` or quoting
unless you run inside a pty.

## Rendering by reply type

| RESP reply | Raw | Human (TTY) |
|---|---|---|
| simple status (`PONG`, `OK`) | `PONG` | `PONG` |
| bulk string (`[]byte`) | the value, unquoted | `"value"` |
| integer | `42` | `(integer) 42` |
| null | `nil` | `nil` |
| empty bulk string | an empty line | `""` |
| error reply | not printed: the command fails (see below) | same; only the interactive REPL prints `(error) …` |
| array | one element per line; nested arrays flattened to lines | `1) …` numbered, nested with `1) 1) …` prefixes |
| empty array | **no output at all** | no output |

Consequences that trip up parsers:

- An empty array and a command that printed nothing else look identical. Use the exit code, and
  use `EXISTS`, `TYPE` or `DBSIZE` when "no elements" and "failed" must be distinguished.
- A legitimate string `nil` and a null reply are indistinguishable in raw output.
- `HGETALL`, `CONFIG GET`, `XRANGE` and similar pair-returning commands print key and value lines
  alternately with no marker or count.
- Unquoted values may contain `\n`, `\r`, tabs or NUL bytes. A multi-line value spans several
  output lines and cannot be separated from neighbouring elements by line splitting.
- Raw output is not escaped: do not feed untrusted binary values into a context that interprets
  escape sequences.
- `INFO` is a single bulk string with CRLF line endings. Raw printing keeps the `\r`, so split on
  `\n` and strip any trailing `\r` before matching `key:value` lines.
- A bulk or status string containing digits (`"42"`) prints exactly like an integer reply (`42`),
  so raw output alone cannot tell a numeric string from a number. `TYPE` says which it is; a
  string key holding `42` still has type `string`.

## Structured output: JSON via Lua

The CLI has no `--json` or `--csv` flag. To get reliably parseable structure, have the server
serialize with `cjson` inside a script and return it as one string. In raw mode it prints as one
JSON line.

```lua
-- hgetall_json.lua
return cjson.encode(redis.call('HGETALL', KEYS[1]))
```

```bash
redis --eval hgetall_json.lua user:42 ,
# -> ["name","Ada","role","admin"]
```

`--eval` argument rules (important — they are not the same as `EVAL` on the wire):

- `redis --eval <script.lua> <key>… , <arg>…`
- The standalone `,` token separates KEYS (everything before it, passed as `KEYS[1..n]`) from
  ARGV (everything after it). The number of keys is inferred from the position of `,`.
- Without a `,` token, **every** argument is a key and `ARGV` is empty.
- The comma must be its own shell word (surrounding spaces). `redis --eval s.lua a,b` is a single
  key named `a,b`.
- Keys and args are strings; scripts must convert numbers with `tonumber(ARGV[n])`.
- Quirk: when a `,` is present, the CLI sends one extra empty string at the end of `ARGV` (the
  separator shifts arguments left and leaves the last slot empty). Never rely on `#ARGV`; read
  `ARGV[1]`, `ARGV[2]`, … explicitly. The `KEYS` list and key count are correct.
- The script file is read relative to the current working directory.
- A script error exits `1` with `Error: run eval script: eval script: <server error>`; a missing
  file gives `Error: run eval script: read eval file: …`.

`cjson` notes: an empty Lua table encodes as `{}`, not `[]`; build arrays with consecutive integer
indices (`t[1]=…, t[2]=…`) to get a JSON array. Numbers become JSON numbers, so large integers can
lose precision beyond 2^53 — return them as strings with `tostring` when exactness matters. Invalid
UTF-8 bytes are not escaped, they are emitted raw inside the JSON string, so a strict parser may
reject the result; fetch binary values separately.

A JSON wrapper around a single key:

```lua
-- value_json.lua
local t = redis.call('TYPE', KEYS[1])
local typ = t['ok'] or t
if typ == 'string' then
  return cjson.encode({type = typ, value = redis.call('GET', KEYS[1])})
elseif typ == 'hash' then
  return cjson.encode(redis.call('HGETALL', KEYS[1]))
else
  return cjson.encode({type = typ})
end
```

```bash
redis --eval value_json.lua user:42 ,
```

The trailing comma is optional when you pass no `ARGV`: `redis --eval value_json.lua user:42`
also works, because a lone argument is treated as a key.

## SCAN without `KEYS`

This CLI has no `--scan` helper, so iterate with the command:

```bash
cursor=0
while :; do
  out=$(redis --raw SCAN "$cursor" MATCH 'session:*' COUNT 500) || exit 1
  cursor=$(printf '%s\n' "$out" | head -n 1)
  printf '%s\n' "$out" | tail -n +2
  [ "$cursor" = "0" ] && break
done
```

The first output line is the next cursor (a bulk string, printed raw); the remaining lines are
keys. An empty page still prints its cursor. The cursor is opaque: never parse it as a number,
just compare it with `0`. Duplicates are possible between pages; deduplicate if the output matters.

## Errors and exit codes

| Situation | Exit | Stderr | Stdout |
|---|---|---|---|
| command succeeded | `0` | empty | the reply (or nothing for an empty array) |
| server error reply (`ERR`, `WRONGTYPE`, `NOSCRIPT`, …) | `1` | `Error: run command: execute command: <error>` | empty |
| dial / auth / TLS / SSH failure | `1` | `Error: dial: dial: dial URL: <cause>` (cluster: `Error: dial: load cluster slots: <cause>`) | empty |
| cluster error | `1` | `Error: run command: execute command: MOVED …` or `Error: run command: execute command: too many cluster redirects` | maybe partial |
| config or profile problem | `1` | `Error: load config: …` / `Error: resolve profile: …` | empty |
| Lua script failure | `1` | `Error: run eval script: …` | empty |
| unknown flag, bad value | `1` | `Error: unknown flag: …` (usage suppressed) | empty |
| `--help`, `-v`, `completion` | `0` | empty | usage / version / script |

Practical rules:

- Branch on the exit code first, then read stderr. The last `Error:` line carries the cause; dial
  errors nest the real reason at the end (`dial: dial: dial URL: <cause>`).
- Stdout is not a success signal. A command that printed nothing and exited `0` succeeded.
- `--ssh-mode external` writes a log line like
  `2026/11/14 18:55:08 INFO starting external ssh tunnel command="ssh -N -D …"` to stderr before
  the command runs. Ignore non-`Error:` stderr lines, or silence them with `2>/dev/null` when you
  only care about failures.
- HTTP-style prefixes such as `ERR`, `WRONGTYPE`, `MOVED`, `ASK`, `CLUSTERDOWN`, `LOADING`,
  `MISCONF`, `NOAUTH`, `WRONGPASS` are server-side conditions, not client bugs. Report them with
  their meaning and the server's message.
- Command names are case-insensitive, so `info server` and `INFO SERVER` are equivalent.

## Streaming and hanging commands

- `MONITOR` is special-cased: it subscribes and prints every command line until the process is
  killed or the connection breaks — it **never returns on its own** and does not print an `OK`
  header. Always run it under a timeout (`timeout 10 redis MONITOR`) and never leave it in a
  foreground agent step.
- `SUBSCRIBE` / `PSUBSCRIBE` in this CLI send the command, print the subscription confirmation
  array, and exit. They do not stream messages; do not use them to observe a channel. Use
  `PUBSUB CHANNELS` / `PUBSUB NUMSUB` for state instead.
- Blocking commands with an infinite timeout hang until data arrives: `BLPOP key 0`,
  `BRPOP key 0`, `BRPOPLPUSH src dst 0`, `BLMOVE … 0`, `XREAD BLOCK 0 …`, `WAIT 0 …`. Always pass
  a finite timeout or wrap the call in `timeout`.
- `redis` with no arguments starts the REPL and waits on stdin. That is the most common cause of a
  hung agent invocation.
