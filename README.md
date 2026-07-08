# Redis CLI

A Go-based alternative to the official redis-cli application. Fork of [IBM-Cloud/redli](https://github.com/IBM-Cloud/redli).

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
  -P, --profile string             Profile name to connect to (from config file)
  -f, --config string              Config file path
  -u, --uri string                 URI to connect to
  -h, --host string                Host to connect to (default "127.0.0.1")
  -p, --port int                   Port to connect to (default 6379)
  -n, --db int                     Redis database to access
  -c, --cluster                    Force cluster mode
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

### Configuration

See [redis.example.yaml](contrib/redis.example.yaml) for reference.

#### Config File Search Order

1. Path specified by `--config` / `-f` flag
2. `./redis.yaml` (current directory)
3. `$XDG_CONFIG_HOME/redis/redis.yaml`
   - Linux: `~/.config/redis/redis.yaml`
   - macOS: `~/Library/Application Support/redis/redis.yaml`

If no config file is found, a default configuration is used with a `default` profile pointing to `127.0.0.1:6379`.

#### Config Fields

| Field | Description |
|-------|-------------|
| `default_profile` | Default profile name used when `--profile` is not specified |
| `profiles` | Map of profile name to profile config |

Each profile supports the following fields:

| Field | Description |
|-------|-------------|
| `uri` | URI to connect to (overrides host/port/user/password/db/tls) |
| `host` | Host to connect to (default `127.0.0.1`) |
| `port` | Port to connect to (default `6379`) |
| `db` | Redis database to access |
| `cluster` | Enable cluster mode |
| `user` | Username (Redis 6+) |
| `password` | Password |
| `tls` | Enable TLS/SSL |
| `sni` | Server Name Indication for TLS certificate verification |
| `insecure` | Disable certificate validation |
| `cacert` | CA certificate file for validation |
| `certb64` | Self-signed certificate string as base64 for validation |
| `ssh` | SSH tunnel configuration |

#### `ssh`

| Field | Description |
|-------|-------------|
| `host` | SSH bastion host address |
| `port` | SSH port (default `22`) |
| `user` | SSH user name |
| `identity_file` | Path to SSH private key. If not specified, the following default keys are tried in order: `~/.ssh/id_ed25519`, `~/.ssh/id_ecdsa`, `~/.ssh/id_dsa`, `~/.ssh/id_rsa` |

Use `--profile` / `-P` to select a profile. CLI flags override profile values.

```bash
redis PING                    # uses default_profile
redis -P staging PING         # uses staging profile
redis -P staging --host 10.0.0.6 PING  # flag overrides profile
```

### Shell Completion

`redis` supports shell completion for Bash, Zsh, and Fish.

```bash
# Load completion for current session
source <(redis completion bash)  # or zsh / fish
```

To install permanently, run `redis completion <shell> --help` for instructions.

### Examples

```bash
# Connect to local Redis (uses default profile)
redis

# Connect with a named profile
redis -P staging

# Connect with URI
redis -u redis://user:password@host:6379/0

# Connect with TLS
redis --tls -h my.redis.host -p 6379 -a mypassword

# Connect via SSH tunnel
redis --ssh user@ssh-host -h my.redis.host -p 6379

# Connect to a Redis Cluster and follow slot redirects
redis -c -h 127.0.0.1 -p 7000

# Execute a command
redis INFO KEYSPACE

# Evaluate a Lua script
redis --eval script.lua key1 key2 , arg1 arg2
```

Be aware of interactions with wild cards and special characters in the shell; quote and escape as appropriate.

## License

Redis CLI is licensed under the [Apache License 2.0](https://opensource.org/licenses/Apache-2.0). See [LICENSE](LICENSE) for more details.

The `commands.json` file is by Salvatore Sanfilippo and is distributed under a [CC-BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/) license (see [Copyright](https://github.com/antirez/redis-doc/blob/master/COPYRIGHT)).
