# Connections

How `redis` decides where to connect, and how to fix it when it cannot.

## Precedence

The connection is assembled field by field:

1. **Flags** you pass on the command line.
2. **Profile** selected with `-P/--profile`; without `-P`, the profile named by
   `default_profile` is used.
3. **Built-in defaults**: profile `default` → `127.0.0.1:6379`, database `0`, no TLS, no SSH.

For each field the flag wins over the profile value. The one exception is `uri`: when a profile or
`-u` supplies a URI, that URI is used on its own, and `--host`, `--port`, `-n`, `--user`,
`--password` and `--tls` no longer change the address, database or auth. TLS then depends on the
URI scheme (`rediss://`), not on `--tls`.

Profile lookup errors are fatal: `Error: resolve profile: profile not found: <name>` (the `-P`
name, or a `default_profile` that names a missing profile).

## Config file

Search order:

1. `-f/--config <path>` — the file must exist and parse; otherwise
   `Error: load config: read config: …`.
2. `./redis.yaml` in the current directory.
3. `$XDG_CONFIG_HOME/redis/redis.yaml` — Linux `~/.config/redis/redis.yaml`, macOS
   `~/Library/Application Support/redis/redis.yaml`.
4. Nothing found → the built-in `default` profile is used (no error).

The first match in that order wins; `./redis.yaml` deliberately shadows the user config.
The config directory is created (mode `0755`) even when no file exists.

Example, mirroring the repository's `contrib/redis.example.yaml`:

```yaml
default_profile: local

profiles:
  local:
    host: 127.0.0.1
    port: 6379
    db: 0
  production:
    uri: rediss://prod-redis.example.com:6379
    user: default
    password: secret
    cluster: true
    tls: true
    sni: redis.example.com
    insecure: false
    cacert: /path/to/ca.crt
    ssh:
      host: bastion.example.com
      port: 22
      user: deploy
      identity_file: ~/.ssh/id_ed25519
  staging:
    host: 10.0.0.5
    port: 6380
    password: stagingpass
    db: 1
    tls: true
    insecure: true
```

`production` illustrates the URI form: when `uri` is set, the separate `user`, `password` and
`tls` fields are ignored, so credentials and `rediss://` belong in the URI itself (the profile's
`ssh` block still applies).

Profile fields:

| Field | Meaning |
|---|---|
| `uri` | Redis URI; when set it replaces host/port/db/user/password/tls below |
| `host`, `port` | address used when `uri` is empty (`127.0.0.1`, `6379`) |
| `db` | database index (`0`); must be `0` in cluster mode |
| `cluster` | force cluster mode (`false`) |
| `user`, `password` | ACL user (Redis 6+) and password |
| `tls` | connect with TLS |
| `sni` | TLS server name override (certificate hostname) |
| `insecure` | skip certificate verification (avoid) |
| `cacert` | PEM file with the CA certificate to trust |
| `certb64` | base64 of that same PEM, for environments that cannot ship a file |
| `ssh` | SSH tunnel block, below |

`ssh` block:

| Field | Meaning |
|---|---|
| `host` | bastion host; in `external` mode also a `Host` alias from `~/.ssh/config` |
| `port` | bastion port (default `22`); leave unset in `external` mode to inherit |
| `user` | bastion user; leave unset in `external` mode to inherit |
| `identity_file` | private key file; leave unset in `external` mode to inherit |
| `mode` | `builtin` (default, in-process SSH client) or `external` (system `ssh -N -D`) |
| `options` | extra `ssh` arguments, passed through as-is; `external` only |

The file may contain plaintext passwords. Restrict it: `chmod 600 redis.yaml`.

## URIs

`-u/--uri` accepts the provisional Redis URI scheme:

```text
redis://[user[:password]@]host[:port][/db]
rediss://[user[:password]@]host[:port][/db]
```

- `redis://` is plaintext; `rediss://` enables TLS and is the only way to get TLS when using a URI
  (`--tls` is ignored in that case).
- The password in a URI must be percent-encoded (`@`, `:`, `/`, `#`, `%` in a password will
  otherwise be parsed as structure). Passing `-a` instead escapes it for you.
- When a password is given without a user, the user defaults to `default` (Redis 6+ ACLs).
- The `/db` path selects the database.

```bash
redis -u 'redis://:secret@10.0.0.5:6379/2' PING
redis -u 'rediss://default:secret@prod-redis.example.com:6379' INFO SERVER
redis -u 'redis://cache.internal:6379' -c PING      # cluster seed
```

When no URI is used, the address is built from `-h/--host` (default `127.0.0.1`), `-p/--port`
(default `6379`) and `-n/--db` (default `0`).

## TLS

| Flag | Effect |
|---|---|
| `--tls` | TLS with the system roots and SNI = host (flag mode only) |
| `-s/--sni <name>` | override the certificate hostname (e.g. connecting through a proxy) |
| `--cacert <file>` | add the PEM CA certificate to the trusted roots |
| `--certb64 <base64>` | same CA material, base64-encoded |
| `--insecure` | skip certificate and hostname verification — only with explicit user consent |

`--cacert`/`--certb64` resolution: flag takes precedence, then `REDIS_CACERT` / `REDIS_CERTB64`
environment variables, then the profile (`--cacert` beats `--certb64` when both are set). The PEM is
added as a **root CA**; this client does not present a client certificate for mutual TLS.

Common certificate failures:

- `x509: certificate signed by unknown authority` — self-signed/private CA: pass `--cacert`.
- `x509: certificate is valid for X, not Y` — the endpoint name differs from the certificate:
  use `--sni X`, or `-h X` while keeping the real address in `-u`.
- `tls: first record does not look like a TLS handshake` — plaintext endpoint with TLS, wrong
  `rediss://` on a non-TLS port, or TLS expected on a plain port. Check the server's `tls-port` in
  its config. `TLS handshake timeout` is the same mismatch when the endpoint never answers the
  handshake at all.

## SSH tunnel

```bash
# Built-in SSH client (key authentication only)
redis --ssh user@bastion.example.com -h redis.internal -p 6379 PING
redis --ssh user@bastion.example.com:2222 --ssh-identity-file ~/.ssh/deploy_key -h 10.0.0.5 PING

# System ssh (reads ~/.ssh/config, ssh-agent, prompts, ProxyJump)
redis --ssh bastion-alias --ssh-mode external -h redis.internal -p 6379 PING
```

`--ssh` takes `[user[:password]@]host[:port]`; it overrides the profile `ssh` block. Without
`--ssh`, `--ssh-identity-file` and `--ssh-mode` only patch the profile block (they need one).

- `builtin` (default): in-process SSH client; tries `~/.ssh/id_ed25519`, `id_ecdsa`, `id_dsa`,
  `id_rsa` when no identity file is given; no interactive prompts, so it cannot do password auth,
  keyboard-interactive, `~/.ssh/config` aliases or passphrase-protected keys (an encrypted key
  fails to parse). The password part of `--ssh user:pass@host` is accepted but ignored.
- `external`: runs the system `ssh -N -D` as a SOCKS proxy, so `~/.ssh/config`, `ssh-agent`,
  `ProxyCommand`, `ProxyJump` and password prompts behave like a normal `ssh` invocation. Use a
  `Host` alias and leave `port`, `user` and `identity_file` unset. Extra `ssh` arguments go in
  `options`. An `INFO starting external ssh tunnel command="ssh …"` log line is written to
  stderr; it contains no secret but does show the identity file path.
- The tunnel is opened once per invocation and shared by all cluster node connections, then closed
  on exit.
- Always keep `-h`/`--host` pointing at the **target inside** the bastion's network; the bastion
  address is only in `--ssh`.

## Cluster

```bash
redis -c -h seed-node -p 7000 CLUSTER INFO
redis -c -u 'rediss://default:secret@seed.example.com:6379' INFO SERVER
```

- `-c/--cluster` forces cluster mode (otherwise a cluster is treated as a single node and
  `MOVED`/`ASK` replies surface as errors).
- The seed is the normal address/URI; cluster nodes are discovered with `CLUSTER SLOTS` and
  `MOVED`/`ASK` redirects are followed automatically (up to 8 redirects, then
  `Error: run command: execute command: too many cluster redirects`).
- Database `0` only: `redis cluster supports database 0 only: got database N`. A URI path is
  forced to `/0`.
- Redirect targets are dialed directly (through the same tunnel when `--ssh` is used), so the
  client host must be able to reach every node's advertised address.
- Dialing a cluster uses `CLUSTER SLOTS`, so a seed that is not a cluster node fails with
  `Error: dial: load cluster slots: ERR This instance has cluster support disabled`; malformed
  replies fail with `parse cluster slots: …`. Both mean wrong seed, mid-failover or a proxy that
  rewrote the reply.

## Troubleshooting

| Error prefix | Meaning and next step |
|---|---|
| `Error: dial: dial: dial URL: dial tcp …: connect: connection refused` | Nothing listening at host:port. Check the address, the server, the firewall, and whether the tunnel is up. |
| `Error: dial: dial: dial URL: …: i/o timeout` | Wrong host/port, blocked network, or a VPN/bastion path that is not open. |
| `Error: dial: dial: dial URL: WRONGPASS invalid username-password pair` (or `NOAUTH`, or `ERR invalid password` on older servers) | Credentials rejected. Recheck the profile and ACL user; `redis PING` is the smallest probe. |
| `Error: dial: dial: dial URL: x509: certificate signed by unknown authority` | Private CA; use `--cacert` / `REDIS_CACERT`. |
| `Error: dial: create SSH tunnel: …` | Bastion unreachable or key rejected. In `builtin` mode confirm the key and `--ssh user@host`; in `external` mode run the same `ssh` command manually to see the prompt/error. |
| `Error: dial: create SSH tunnel: unsupported ssh mode: <x>` | `--ssh-mode` must be `builtin` or `external`; an empty value means `builtin`. |
| `Error: load config: read config: …` | An explicitly passed `-f` file is missing or invalid YAML. Profiles are not auto-created. |
| `Error: resolve profile: profile not found: <name>` | `-P` typo, or `default_profile` names a missing profile. List the profile keys in the file. |
| `Error: dial: redis cluster supports database 0 only: got database N` | Remove `-n`/`--db`, or set it to `0`; cluster has one logical database. |
| `Error: run command: execute command: MOVED …` | Cluster mode was not enabled: add `-c`, or use a URI that the tool treats as a single node. |

Dial failures print the nested reason, so read the last segment of the message before acting:
`Error: dial: dial: dial URL: <real cause>` for single-node connections, or
`Error: dial: <stage>: <real cause>` (for example `load cluster slots:`) in cluster mode.

## Quick checks before a session

```bash
redis -v                                  # which client, which build
redis --help                              # authoritative flag list
redis PING                                # reachability + auth + TLS in one command
redis INFO SERVER                         # target identity: version, run_id, port
redis CLUSTER INFO                        # only when the target is a cluster
```
