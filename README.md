# Redis CLI

A Go-based alternative to the official redis-cli application.

> **Fork of [IBM-Cloud/redli](https://github.com/IBM-Cloud/redli)**

## Installation

```shell
go install github.com/aaronjheng/redis-cli/cmd/redis@latest
```

If you need the newest `master` commit immediately (without relying on branch-resolution cache), install by resolved commit SHA:

```shell
go install github.com/aaronjheng/redis-cli/cmd/redis@$(git ls-remote https://github.com/aaronjheng/redis-cli.git refs/heads/master | cut -f1)
```

## Usage

```text
redis [<flags>] [<commands>...]

Flags:
  -u, --uri string                 URI to connect to
  -h, --host string                Host to connect to (default "127.0.0.1")
  -p, --port int                   Port to connect to (default 6379)
  -n, --db int                     Redis database to access
  -r, --user string                Username to use when connecting. Supported since Redis 6.
  -a, --password string            Password to use when connecting
      --tls                        Enable TLS/SSL
  -s, --sni string                 Server Name Indication for TLS certificate verification
      --insecure                   Disable certificate validation
      --cacert string              CA certificate file for validation
      --certb64 string             Self-signed certificate string as base64 for validation
      --ssh string                 SSH tunnel connection URI. Format: [user[:pass]@]host[:port]
      --ssh-identity-file string   SSH identity file
      --raw                        Produce raw output
      --eval string                Evaluate a Lua script file, follow with keys a , and args
  -v, --version                    Print version
      --help                       Help for redis
```

### URI

The URI follows the format of [the provisional IANA spec for Redis URLs](https://www.iana.org/assignments/uri-schemes/prov/redis), with the option to denote a TLS secured connection with the `rediss://` protocol.

### Shell Completion

`redis` supports shell completion for Bash, Zsh, and Fish.

```bash
# Load completion for current session
source <(redis completion bash)  # or zsh / fish
```

To install permanently, run `redis completion <shell> --help` for instructions.

### Examples

```bash
# Connect to local Redis
redis

# Connect with URI
redis -u redis://user:password@host:6379/0

# Connect with TLS
redis --tls -h my.redis.host -p 6379 -a mypassword

# Connect via SSH tunnel
redis --ssh user@ssh-host -h my.redis.host -p 6379

# Execute a command
redis INFO KEYSPACE

# Evaluate a Lua script
redis --eval script.lua key1 key2 , arg1 arg2
```

Be aware of interactions with wild cards and special characters in the shell; quote and escape as appropriate.

## License

Redis CLI is licensed under the [Apache License 2.0](https://opensource.org/licenses/Apache-2.0). See [LICENSE](LICENSE) for more details.

The `commands.json` file is by Salvatore Sanfilippo and is distributed under a [CC-BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/) license (see [Copyright](https://github.com/antirez/redis-doc/blob/master/COPYRIGHT)).
