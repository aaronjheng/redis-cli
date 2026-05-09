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
redis-cli [<flags>] [<commands>...]

Flags:
  -u, --uri string              URI to connect to
  -H, --host string             Host to connect to (default "127.0.0.1")
  -p, --port int                Port to connect to (default 6379)
  -r, --redisuser string        Username to use when connecting. Supported since Redis 6.
  -a, --auth string             Password to use when connecting
  -n, --ndb int                 Redis database to access (default 0)
      --tls                     Enable TLS/SSL
  -s, --servername string       ServerName for TLS certificate verification
      --skipverify              Don't validate certificates
      --certfile string         Self-signed certificate file for validation
      --certb64 string          Self-signed certificate string as base64 for validation
      --raw                     Produce raw output
      --eval string             Evaluate a Lua script file, follow with keys a , and args
      --ssh string              SSH tunnel connection URI. Format: [user[:pass]@]host[:port]
      --ssh-identity-file string SSH identity file
      --version                 Print version information
```

### URI

The URI follows the format of [the provisional IANA spec for Redis URLs](https://www.iana.org/assignments/uri-schemes/prov/redis), with the option to denote a TLS secured connection with the `rediss://` protocol.

### Examples

```bash
# Connect to local Redis
redis-cli

# Connect with URI
redis-cli -u redis://user:password@host:6379/0

# Connect with TLS
redis-cli --tls -H my.redis.host -p 6379 -a mypassword

# Connect via SSH tunnel
redis-cli --ssh user@ssh-host -H my.redis.host -p 6379

# Execute a command
redis-cli INFO KEYSPACE

# Evaluate a Lua script
redis-cli --eval script.lua key1 key2 , arg1 arg2
```

Be aware of interactions with wild cards and special characters in the shell; quote and escape as appropriate.

## License

This project is forked from [IBM-Cloud/redli](https://github.com/IBM-Cloud/redli), (c) IBM Corporation 2018-2024. All rights reserved.

Licensed under the [Apache License 2.0](LICENSE).

The `commands.json` file is by Salvatore Sanfilippo and is distributed under a [CC-BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/) license (see [Copyright](https://github.com/antirez/redis-doc/blob/master/COPYRIGHT)).
