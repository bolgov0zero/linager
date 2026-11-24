# Linager architecture

Status: implemented in 0.1; sections 12a–12e describe what was built and where the implementation departed from the initial plan. Source requirements: `docs/kickoff/PROMPT.md`, decisions: `docs/decisions.md`, interface: `docs/design/ui-spec.md`.

## 1. Overall scheme

```
 browser ──HTTP(S)──▶ Caddy/nginx ──HTTP──▶ linager server (admin panel, API, WS/SSE) ──▶ PostgreSQL (localhost)
                                                   ▲
 linager agent (root, Debian/Ubuntu) ──WSS + mTLS, outbound──┘  (separate agent port)
```

- A single `linager` binary with subcommands: `server`, `agent`, `install`, `update`, `backup`, `version`.
- Two independent listening ports on the server, both set during installation with no default values:
  - **admin panel port** — plain HTTP. A reverse proxy (Caddy) is expected on this same server or a neighboring one; the same mode is used for closed and local networks;
  - **agent port** — the Linager server's own TLS (self-signed certificate, the fingerprint is pinned on the agent) and mTLS after enrollment.
- In 0.1, agents connect directly to the agent port, not through a proxy: mTLS will not pass through a regular HTTP proxy. See open question 1.

## 2. Technologies

| Area | Choice | Reason |
|---|---|---|
| Language | Go 1.23+, amd64; arm64 for the agent | customer's decision |
| CLI | `spf13/cobra` | subcommands and flags for non-interactive mode |
| HTTP/WS | `net/http` + `coder/websocket` | minimal dependencies |
| DB | `jackc/pgx/v5`, queries written by hand in `server/store` | no ORM and no code generation |
| Migrations | `pressly/goose` embedded (`go:embed`), `up` only | migrations go forward only |
| systemd | `coreos/go-systemd` + D-Bus (`godbus`) | start/stop/restart/enable without a shell |
| nftables | CLI `nft -j` and `nft -c -f` for checking | stable JSON, atomic application |
| Docker | Docker Engine API over a unix socket | no CLI needed |
| Terminal | `creack/pty` on the agent, xterm.js in the web UI | |
| Installer (TUI) | `charmbracelet/bubbletea` + `lipgloss` + `bubbles` | step-by-step wizard, progress, spinners |
| Frontend | Vite + Svelte 5 + TypeScript, charts drawn by hand in SVG | the mockup is written in plain JS/CSS; the framework is kept minimal; styles are carried over from the mockup as-is |
| i18n | JSON dictionaries `ru.json`, `en.json` for the web UI; `ru`/`en` for the installer and CLI | |
| Passwords | argon2id (`golang.org/x/crypto`) | |
| TOTP | `pquerna/otp` | |
| Release signing | minisign-compatible Ed25519 | see section 9 |

Choosing Svelte and sqlc is a recommendation; it can be replaced before stage 3 begins.

## 3. Repository structure

```
cmd/linager/            main, subcommand registration
server/                 HTTP API, WS hub, scheduler, update rollout
agent/                  agent core and modules (metrics, terminal, services, apt, ...)
internal/
  protocol/             messages, protocol versions, signatures
  pki/                  internal CA, certificates, fingerprints, verification codes
  config/               reading and validating server.yaml / agent.yaml
  release/              manifest, signature verification, download
  i18n/                 shared CLI dictionaries
installer/              installation wizard (TUI) and non-interactive mode
install.sh              thin bootstrapper
migrations/             SQL, forward only
web/                    frontend (built into web/dist, go:embed)
docs/{ru,en}/           documentation; docs/design/ — mockup, docs/kickoff/ — starter package
.github/workflows/      CI, releases
packaging/              systemd templates, Windows installers, Caddy and nginx examples
```

## 4. Protocol and agent enrollment

### 4.1 Enrollment
1. `linager install` in "Client" mode asks for the server's `host:port`, generates an Ed25519 key (`/etc/linager/agent.key`, 600), and outputs the public key and the **enrollment key** `XXXX-XXXX`: the first 40 bits of `SHA-256(pubkey)` in Base32 (Crockford).
2. The agent connects to the agent port over TLS. The server fingerprint is checked with the `--fingerprint` flag; without the flag, the agent shows the fingerprint and asks for explicit confirmation.
3. The agent sends an enrollment request: public key, host name, address, OS and agent versions, protocol version. The server creates a `pending` record (lifetime 15 minutes, request limit per address).
4. **Verification code** (4–6 characters) = truncated `HMAC(SHA-256(server_cert) ‖ agent_pubkey, nonce)`. It is shown on the agent and in the admin panel next to the fingerprint: if the server is spoofed, the codes will differ.
5. The administrator enters the enrollment key from the agent's console in the admin panel (the "Awaiting approval" screen). The server compares it with the key derived from the public key in the request. Several failed attempts block the record.
6. The server issues the agent an Ed25519 client certificate from the internal CA (valid for 1 year, auto-renewed through the channel) and hands it over together with the CA certificate. From then on, only mTLS is used.

### 4.2 Working channel
- WSS, an outbound connection from the agent, automatic reconnection with exponential backoff and jitter.
- Message envelope: `v` (protocol version), `id`, `seq`, `ts`, `type`, `payload`, `sig` (the sender's Ed25519 signature). The server signs commands with a separate command-signing key; the agent accepts only signed and fresh commands (replay protection via `seq` and `ts`).
- Versioning: the server keeps the supported protocol versions and works with the previous version of the agent; a version below the minimum is marked "not supported".
- Message types: `hello`, `metrics`, `cmd`, `cmd.result`, `stream.open/data/close` (terminal, logs), `update.*`, `ping`.
- Metrics are sent in batches every 5 s; if the connection is lost, the agent buffers up to 1 hour (ring buffer) and sends the backlog on reconnection.

### 4.3 Network and firewall rollback
Performed by the agent, and only by the agent:
1. The draft is stored on the server; nothing is sent to the agent until "Apply".
2. "Apply": the agent makes a copy of the current configuration, writes a rollback journal to `/var/lib/linager/pending/` (it survives an agent restart and a reboot), applies the change (`nft -c` and then `nft -f`; for the network, `ifup/ifdown` with a backup of `interfaces`), and starts a 30-second timer.
3. Confirmation arrives over the channel; without it (or if the connection is lost), the agent restores the configuration. Fallback mechanism: a one-shot `systemd-run --on-active=35s linager agent rollback`, in case the agent process itself crashes.
4. The confirmed configuration is written to `/etc/nftables.conf` and to the network configuration of its backend: `/etc/network/interfaces` (ifupdown, Debian) or `/etc/netplan/*.yaml` (netplan, Ubuntu; applied using a `netplan try`-like scheme with the agent's own timer). The agent detects the backend automatically; the `network.Backend` interface layer is shared.
5. The server calculates warnings before application based on the description of the changes (port 22, the primary interface) and on the address from which the agent is connected.

## 5. Database

PostgreSQL is local only: access via a unix socket or `127.0.0.1`, `scram-sha-256`, the role `linager` with a random password. The DSN is in `/etc/linager/server.yaml` (600, owner `linager`). Migrations are applied at server startup under a lock (`pg_advisory_lock`).

Main tables (the first migration is stage 1):

- `users`, `roles`, `user_roles` — users and roles are defined from the start; on startup there is one user with the `admin` role. Fields: `password_hash` (argon2id), `totp_secret`, `totp_enabled`, `failed_attempts`, `locked_until`.
- `sessions` — id (token hash), `user_id`, IP, User-Agent, `expires_at`, CSRF secret.
- `servers` — identifier, name, `agent_pubkey`, certificate fingerprint, status (`pending/approved/revoked`), OS, agent and protocol versions, enabled modules, `last_seen`.
- `enrollments` — enrollment requests: pubkey, enrollment key, verification code, expiry, number of attempts.
- `dashboards`, `dashboard_items` — the dashboard and the order of cards (`position`); cards are added and removed only from the dashboard.
- `commands` — the command queue for agents: type, parameters, status, result, expiry, author.
- `audit_log` — who, what, where, when, result, IP. Written for every administrator action.
- `releases`, `update_runs`, `update_steps` — release cache and update history.
- `settings` — key-value pairs (metric retention periods, update channel).
- Metrics: `metrics_raw` (5 s), `metrics_1m`, `metrics_5m`, `metrics_15m`, `metrics_1h`, `events` (reboots, service failures).

### Metrics
- Partitioning with `PARTITION BY RANGE (ts)`: raw — hourly partitions, 1m — daily, 5m — weekly, 15m and 1h — monthly. Partitions are created and removed by the server's scheduler (`DROP` of a partition instead of `DELETE`).
- Aggregation is done by the server's scheduler (not `pg_cron`, not TimescaleDB): every minute it rolls up raw → 1m, every 5 minutes 1m → 5m, and so on; for each window it stores min, avg, p95, and max. Aggregates are idempotent (`INSERT ... ON CONFLICT`).
- Default retention: 5 s — 1 hour, 1 min — 7 days, 5 min — 90 days, 15 min — 180 days, 1 hour — 360 days; configurable in `settings`.
- The history API selects the table by period according to the table in `ui-spec.md`. Data remains available for an offline server.
- Writes are done in batches (`COPY`/multi-row insert).

## 6. Server

- Modules: `auth` (login, sessions, TOTP, lockouts, CSRF), `agents` (WS hub, command routing), `api` (REST for the web UI), `live` (SSE/WS dashboard stream), `scheduler` (aggregation, cleanup, release cache), `updater` (rollout), `audit`.
- Reverse proxy mode: `listen` (default `0.0.0.0:<port>`, the port is set during installation), `trusted_proxies` (default `0.0.0.0/0, ::/0`: `X-Forwarded-For/Proto/Host` headers are accepted from anyone; the wizard does not ask about this; the list is narrowed in Settings → Admin panel access, see 12n; the value in `server.yaml` is in force until something is saved in the database), `base_path` (empty by default, edited in `server.yaml`), WebSocket upgrade support, `cookie_secure: auto` (based on `X-Forwarded-Proto`; on a closed network over plain HTTP the cookie is not marked Secure, and the wizard shows a warning).
- Protection: login attempt limits per IP and per user with increasing delays, CSRF token on all state-changing requests, strict cookies (`HttpOnly`, `SameSite=Strict`), CSP headers, `X-Frame-Options`, `Origin` check on WebSocket.
- The service runs as `linager`, with `NoNewPrivileges`, `ProtectSystem=strict`, and `ReadWritePaths` only for `/var/lib/linager`.

### Removing a client from the admin panel
`DELETE /api/servers/{id}` with the body `{"name": "<server name>", "uninstall": true|false}`; the name is repeated for confirmation. Order of operations:
1. If `uninstall` is set and the agent is online, the server sends a signed `agent.uninstall` command; the agent deletes its certificate and state, disables and stops its service, and with `purge` erases the data directory and config (the binary remains, since it may also be the server), and exits with code 78 (systemd does not restart it).
2. The `servers` row is deleted; dashboard items and the command queue are removed by cascade. Metrics and events are linked to the server and are deleted with it (stage 3). Audit log entries remain.
3. The connection is closed. The agent's certificate stops working immediately, because every connection is checked against the `servers` table: the server responds to a revoked agent with 401 and `X-Linager-Reason: revoked`, and the agent stops (code 78).
4. If the agent was offline or the command did not arrive, the record is still deleted; on its next connection the agent receives `revoked` and stops, and if needed the host is cleaned manually with `linager agent uninstall`.
5. The action is written to the audit log (`server.delete`: whether the agent was online, whether the uninstall command was sent).
The same host can be enrolled again: after deletion, it goes through enrollment again.

## 7. Agent

- Runs as root (required for nft, apt, systemd, PTY). Each module is enabled separately in `/etc/linager/agent.yaml`; a disabled module does not register commands, and the server does not show its tab.
- Modules: `metrics`, `terminal`, `services`, `apt`, `processes`, `logs`, `files`, `network`, `firewall`, `docker`. A single interface `Module{Name, Start, Handle(cmd)}`.
- Dangerous actions are confirmed in the admin panel; the server additionally marks them in the command, and the agent rejects them without the mark.
- The `linager-agent.service` service with `Restart=always`.

## 8. Installer

### 8.1 `install.sh`
A thin script (POSIX sh, about 80 lines):
1. Clears the screen and the scrollback buffer before starting (`printf '\033[H\033[2J\033[3J'`).
2. Checks Debian 12/13 or Ubuntu 22.04/24.04/26.04, amd64 or arm64 (64-bit system), root, and the presence of `curl`.
3. Downloads the release and `SHA256SUMS` with `SHA256SUMS.minisig`, verifies the checksum and the signature (the minimal `minisign` from the Debian repository; the public key is embedded in the script).
4. Runs `linager install "$@"`; without a TTY it goes straight to non-interactive mode.
The documentation's safe option: download, read, verify the signature, run.

### 8.2 `linager install` wizard
- A full-screen TUI on a separate terminal screen (alternate screen): the screen shows only the logo, the step, and the input field; command output is not shown.
- Header with the logo and the list of steps (`✓` done, `●` current, `○` upcoming), at the bottom a single progress bar for the stage and one line for the current action. Minimal text, with no explanations where the question is clear.
- Detailed output (apt, postgres) is written to `/var/log/linager-install.log`; on error the wizard shows the last 10 lines and the path to the log, and rolls back the installer's changes where possible.
- Language from `LANG` (ru/en), `--lang` flag. Color scheme from the mockup (one accent; green/yellow/red). Works without color (`NO_COLOR`, non-TTY).
- The first question: **Client or Server**.

**Server.** Questions: agent port (required), admin panel port (required), admin panel mode (Caddy on this server or own reverse proxy; for Caddy, also a domain), administrator login and password (password hidden, confirmation, length check).
Stages with progress: checks (OS, root, systemd, free ports) → PostgreSQL (`apt-get install`) → database and role → configuration and keys (CA, agent-port certificate, command-signing key) → systemd service → administrator → start and check.
Summary: admin panel address, agent certificate fingerprint, a ready-to-use command for the client, an example Caddy block with the address and port filled in, and an nftables hint for the agent port.

**Client.** Questions: the server's `host:port` (required). Stages: connection check → server fingerprint and confirmation (or `--fingerprint`) → key generation → enrollment request. The waiting screen shows the key `XXXX-XXXX`, the verification code, and the fingerprint, and updates until confirmation; then the `linager-agent` service is enabled and started.

**Non-interactive mode:** `linager install server --agent-port N --admin-port N --admin-mode caddy|proxy [--domain D] --admin-user U --admin-password-file F` and `linager install agent --server HOST:PORT --fingerprint SHA256:...`. Without the required flags the command fails with a clear message; there are no default port values.

## 9. Signing and updates

- **Release signing: minisign-compatible Ed25519.** One embedded public key, verification in pure Go with no external dependencies; `minisign` is available in the Debian repository for manual verification. Cosign was rejected: it pulls in the Sigstore chain and network checks, which are unnecessary for closed networks.
- Release: `linager_linux_amd64` and `linager_linux_arm64`, the files of the Windows agent and of the application, `SHA256SUMS`, `SHA256SUMS.minisig`, `manifest.json` (version, channel, minimum agent version, migration version, changelog), with a signature.
- The secret signing key is stored in a GitHub Actions secret (`MINISIGN_KEY`); it is created by the owner (instructions will be in `docs/`).
- The server caches releases from GitHub (`bolgov0zero/linager`), verifies the signature, and distributes them to agents. Order: the server first, then the agents.
- Server update: signature check → `pg_dump` → service stop → binary backup → atomic replacement (`rename`) → forward migrations → start → health check → automatic binary rollback on failure (forward migrations are not rolled back, which is why `pg_dump` is taken beforehand).
- Agent update: download from the server, signature check, replacement, restart. The new agent confirms its operation with its first successful connection; if there is no confirmation within 60 seconds, a separate timer (`systemd-run`) restores `linager.prev`.
- Offline agents are queued; the rollout order and "stop on error" are handled by the server.

## 10. Security (summary)

TLS on the agent port, mTLS and signed messages; argon2id; TOTP; brute-force lockout; CSRF; audit log of all actions; secrets at 600; minimal service privileges; confirmation of dangerous actions; fingerprint and verification code during enrollment; CSP and strict cookies.

## 11. CI and releases

- GitHub Actions: `go vet`, `golangci-lint`, tests (with PostgreSQL in a service container), frontend build, `linux/amd64` and `linux/arm64` (tests on an arm64 runner too), `SHA256SUMS`, signing, publication to Releases on a `v*` tag.
- Installer test: a matrix of Debian 12/13 and Ubuntu 22.04/24.04/26.04 in containers with systemd (non-interactive mode), plus a TUI check via terminal snapshots.
- Versioning: SemVer, `stable` (tags `vX.Y.Z`) and `beta` (`vX.Y.Z-beta.N`) channels.

## 12. Plan for stages 0–2

### Stage 0. Repository and skeleton
Contents: create the public repository `bolgov0zero/linager` (Apache-2.0, `README`, `LICENSE`, `NOTICE`, `.gitignore`, `CLAUDE.md`, `docs/`); `go.mod`, `cmd/linager` with stub subcommands and `version`; Makefile; `golangci-lint`; CI (tests, linters, amd64 build); release workflow (build, `.deb`, checksums, signing, Releases); `install.sh` with screen clearing and checks; `web/` skeleton (Vite + Svelte, Medeo fonts and their license in the repository, theme switcher).
Done when: CI is green, a signed release is published for tag `v0.0.1`, `linager version` works on Debian 12/13 and Ubuntu 22.04/24.04/26.04, and installing the stub via `install.sh` succeeds.

### Stage 1. DB, protocol, enrollment, authorization
Contents: migrations (section 5), data access layer (sqlc), configuration; `internal/pki` (CA, certificates, fingerprints, enrollment keys, verification codes); `internal/protocol` (envelope, signatures, versions); agent port (TLS, mTLS, WS hub, reconnection); agent: hello, enrollment, key and certificate storage; admin panel: login (argon2id, sessions, CSRF, lockouts), TOTP, the "Awaiting approval" screen, audit; tests for the protocol and enrollment (including fingerprint substitution and a wrong key).
Done when: two processes on one machine complete full enrollment and exchange signed `ping` messages; brute-forcing the enrollment key is blocked; the administrator logs in with TOTP; every administrator action is recorded in `audit_log`.

### Stage 2. Installer
Contents: `install.sh` in production form; the `linager install` wizard (section 8): server and client, non-interactive mode, log; PostgreSQL installation and a role with a random password; configs at 600; the `linager` user; systemd services; reverse proxy mode (trusted proxies, X-Forwarded, WebSocket, base path), Caddy and nginx examples in `docs/`; final screen; tests on clean Debian 12 and 13 in containers.
Done when: on clean Debian 12/13 and Ubuntu 22.04/24.04/26.04, a single wizard run brings up the server and PostgreSQL, the admin panel is reachable through Caddy in a neighboring container, the client enrolls and runs as a service; re-running the wizard does not break the installation (idempotency).

## 12a. Stage 3: metrics and dashboard (implemented)
- Agent: a collector reading `/proc` every 5 s (CPU, RAM, disks, interfaces, load, uptime); every 30 s the number of failed services; every hour the number of apt updates; whether a reboot is required (`/var/run/reboot-required`). If the connection drops, up to an hour (720 samples) is buffered and sent as a batch (`metrics`, a signed message).
- Server: raw data in `metrics_raw` (hourly partitions); rollups `1m/5m/15m/1h` (daily, weekly, and monthly partitions) are computed by the scheduler every minute (repeatable `ON CONFLICT` inserts); outdated partitions are removed (`DROP`); default retention as in the policy table. Higher-level rollups are built from lower ones; the p95 of higher levels is approximate (a percentile over the p95 of the lower level).
- API: `GET /api/dashboard`, `/api/dashboard/available`, `POST/DELETE /api/dashboard/items`, `PUT /api/dashboard/order`, `GET /api/servers/{id}`, `/metrics?period=`, `/metrics.csv`, `GET /api/live` (SSE: `metrics`, `status`, `dashboard`).
- Events (reboot, service failure, agent connection/disconnection) are written to the `events` table and shown on the charts.
- The latest full sample and system information are stored in `servers`, so after a server restart, and for offline servers, the last known values are shown.

## 12b. Stage 4: terminal, services, apt, processes, logs (implemented)
- Protocol: request-response (`cmd` / `cmd.result`) and streams (`stream.open/data/input/resize/close`) over the signed channel; the server checks dangerous actions and sets the `dangerous` flag only after the administrator confirms; the agent rejects them without the flag.
- Agent: modules are enabled in `/etc/linager/agent.yaml` (`modules:`); the agent reports the list of enabled modules on connection; tabs of disabled modules respond "module disabled". Rebooting is always available.
- Terminal: PTY (`creack/pty`) running `bash -l` as root, up to 5 sessions per agent and 3 per server in the admin panel; opening and closing a session are recorded in the audit log (data volume, duration); keystrokes are not recorded.
- Services: `systemctl` (not D-Bus: fewer dependencies, the same result); unit names are validated; `stop/restart/disable` for `ssh`, `sshd`, and `linager-agent` require confirmation.
- apt: the list of updates comes from the simulation `apt-get -s upgrade --with-new-pkgs` (security by source); installation and checking are streamed with a live log; one apt operation at a time. Besides the text, the stream carries events: lines of `\x1e` + JSON (`queue`, `pkg`, `progress`). The agent runs apt with `-o APT::Status-Fd=3`, reads `pmstatus`/`pmerror`/`dlstatus` from that descriptor and keeps the package states itself; events are stored in the job apart from the log (the 1 MiB log limit does not trim them) and replayed on every attach. The browser cuts the event lines out of the console text.
- Processes: top by CPU or memory from `/proc`; process termination (SIGTERM/SIGKILL) with confirmation; PID 1 and the agent itself are protected.
- Logs: `journalctl -f` streamed with filters by unit, kernel, and priority.
- Admin panel CSP: `style-src 'self' 'unsafe-inline'` (xterm.js inserts styles at runtime); scripts remain `'self'` only.

## 12c. Stage 5: files, network, firewall, Docker (implemented)
- Rollback on the agent: `agent/pending.go`. Before applying a change, the agent writes the journal `/var/lib/linager-agent/pending.json`, starts a 30-second timer, and only then changes the system. Confirmation makes the change permanent; without confirmation (or if the connection is lost), the agent rolls it back by itself. After the agent restarts, the journal is processed again: an expired change is rolled back immediately.
- Network (ifupdown, including ifupdown2): the agent builds a candidate `interfaces` next to the original, switches the interface (`ifdown -i old`, `ifup -i candidate`), and leaves the original `/etc/network/interfaces` untouched until confirmation; rollback does the reverse. The static/DHCP mode, address, gateway, MTU, and state can be edited; other options (dns-*, hwaddress, scripts) are preserved. Interfaces from `source` files and virtual interfaces (docker, veth, wg) are view-only. **netplan (Ubuntu) is view-only in this version**: editing is not enabled until it can be tested on a live system.
- Firewall: only `table inet filter` is managed (chains input, forward, output); foreign tables (Docker and others) are not changed. Application is atomic (`delete table` + a new table in a single `nft -f` script), rules are checked with `nft -c`; after confirmation, the table is written to `/etc/nftables.conf`, replacing only the `table inet filter` block. Warnings (closing port 22, no established-connection rule, output policy drop) are calculated by the server.
- Drafts are stored in the panel database (`drafts`); an unconfirmed change is stored in `pending_changes`; the panel shows a counter, but the rollback timer is run by the agent.
- Files: listing, editing text files up to 1 MB with change checks (by modification time), creating, uploading and downloading via streaming over the agent channel, deletion with confirmation; top-level system directories cannot be deleted.
- Docker: Engine API over the socket; the module is enabled only if the socket exists; containers, images, logs, and pull via streaming. Docker and netplan have not been tested on live machines (not present in the test environment).

## 12d. Stage 6: updates, backups, log (implemented)
- Signing: minisign-compatible Ed25519, verification in pure Go (`internal/release`, algorithms `Ed` and `ED`, trusted comment verification); the public key is embedded (`release.PublicKey`, can be overridden with ldflags). The release manifest (`manifest.json` + `.minisig`) contains the version, channel, SHA-256 and size (of the amd64 binary, the fields of earlier releases), the `assets` map (per architecture: name, SHA-256 and size of the `linager_linux_<arch>` binary), number of migrations, `min_agent`, and the changelog.
- Source: GitHub Releases or a mirror (`update.source`); releases are downloaded to the server cache at `/var/lib/linager/releases/<version>/`; the signature and checksum are verified again before each distribution.
- Server: the privileged `updater` (root) is invoked by a systemd path unit watching the request file from the admin panel; the steps verify, backup, stop, replace, migrate, and start are written to `update-status.json` with the time each began and ended (`started_ms`, `finished_ms`); on failure the binary is restored. History is kept in `update_runs`. The panel is served by the very service that the update stops from stop to start, so it cannot read the status meanwhile: the progress dialog shows the steps that had not finished at its last answer as "going on", with a seconds counter and an explanation, and when the server is back it shows the real result and how long each step took.
- Agents: the server distributes the release over mTLS (`/v1/release/...`), command `update.apply`, messages `update.event`; the agent verifies the signature, creates `linager.prev`, replaces the binary, and arms the rollback timer (`systemd-run`, 70 s); the new version confirms itself by connecting. Rollout manager (all at once, one by one, first one then the rest; stop on error; offline agents queued in `agent_updates`).
- Audit log: `GET /api/audit` with filters (comma-separated action prefixes, result, user, search) and the "Audit log" page.
- Backups: `linager backup` (pg_dump + configuration + keys, permissions 600, rotation), a daily timer, and a pre-update backup.
- Notifications (Telegram, email) are not implemented, by the owner's decision.

## 12f. Users, passkeys, snippets, container updates (0.1.40)
- Migration `00007`: `recovery_codes` (SHA-256 hashes of one-time codes), `webauthn_credentials` (passkeys, a serialized `webauthn.Credential`), `users.webauthn_handle` (a random user id for the authenticator), `snippets` (shared by all).
- Sign-in: `POST /api/login` takes either the app code (6 digits) or a recovery code in the `totp` field; a right password without a code answers `401 totp_required` when 2FA is on. Passkey sign-in: `POST /api/login/passkey/begin|finish` (discoverable credentials, user verification required, a single-use challenge kept in memory for 5 minutes, the origin must match `Host`); the `go-webauthn` library.
- Users and account: `/api/users` (list, create, delete, reset password and 2FA), `/api/me/password`, `/api/me/recovery-codes`, `/api/me/passkeys`. You cannot delete yourself; a password change ends the other sessions; everything is audited. The sign-in rate is set by `admin.login_rate`.
- Snippets: `/api/snippets` (CRUD, order, `POST /{id}/ran` audits a run).
- Agent: the stream `docker.update` (`kind=docker-update`): recreates the container through the Docker API with a rollback, or runs `docker compose pull` and `up -d --no-deps` for a compose project; `docker.state` reports a `compose` flag.

## 12g. The Security tab (0.1.42)
- Agent: the `security` module, the `security.state` command without parameters (it reads fixed sources only, no outside input). Sources: `sshd -T` or `sshd_config`, `journalctl -t sshd -t sshd-session --since "7 days ago" -o short-unix` (parsed as a stream, at most 2 million lines), `/var/log/auth.log` as a fallback, `fail2ban-client`, `/etc/apt/apt.conf.d`, `/etc/passwd` and `authorized_keys` (fingerprints are computed in Go), `loginctl`. The result is cached for a minute.
- Ending sessions: `POST /api/servers/{id}/security/terminate` (`session` or `user`, `confirm: true`); the agent command `security.terminate` needs the confirmation flag, takes its targets only from its own list of remote `loginctl` sessions and writes `security.session.terminate` to the audit log.
- Server: `GET /api/servers/{id}/security`, a proxy to the agent (40 second timeout). The tab is hidden when the agent did not report the `security` module.

## 12n. Admin panel access (0.1.69)
- Settings → Admin panel access: the mode (off, white list, black list), a list per mode, the switch "only through trusted proxies", the trusted proxies and the access journal. Mockup: `docs/design/next/access.html`.
- The settings are in the `settings` table (key `access`, JSON); the journal is the `access_log` table (migration 00010). While nothing is stored, the trusted proxies come from `admin.trusted_proxies` of `server.yaml`; after the first save the database is the source. `ProxyTrust` is replaced at run time.
- The check is the first layer in front of the admin router (`server/ipaccess.go`, `accessGuard`) and covers the page, the API and WebSockets. The address from `X-Forwarded-For` is used only when the connection comes from a trusted proxy; otherwise it is the address of the connection, so the local network can connect directly. A refused request gets a bare 403. The agent port is not affected.
- Saving applies the same rules as the draft check (`POST /api/access/check`): trust of everybody (`0.0.0.0/0`), a proxy that sends `X-Forwarded-For` but is not in the list, an empty white list, locking yourself out (answer 422 `access_problems`). Changes go to the audit log (`access.update`, `access.retention`).
- The journal holds: a page open (at most once per 10 minutes per address), sign-ins good and failed (taken from the `auth.login` audit action), refused requests (at most once a minute per address). Retention 7, 30 or 90 days and a cap of 50,000 rows.
- The settings are re-read from the database every 5 seconds, so `linager access reset` (turns the restriction off, the lists stay) works without restarting the service. A damaged stored value never locks anybody out: the restriction counts as off and the error goes to the service log.
- Host names (0.1.70, DDNS): an entry of the white or black list may be a name (`server/ipaccess_hosts.go`; `AccessRule.CIDR` holds the lower-case name, the parser tells it from an address; names are refused among the trusted proxies, at most 20 names). The rules are the addresses the name resolved to (`compileAccess(cfg, hostAddrs)`); `decide` is unchanged and never calls DNS on the request path. A background job resolves the names of the current lists once a minute (`LookupNetIP`, a 5 s timeout, up to 16 addresses; unspecified and multicast addresses are dropped) and recompiles the rules. A failed lookup keeps the last good answer, which stays in force for 24 hours and then stops counting. The last answers are stored in `settings` (key `access_hosts`, written when they change and at most every 30 minutes to refresh the time) and are read before the first request, so a restart while DNS is down does not change decisions. The draft check (`POST /api/access/check`) resolves new names itself (at most once per 30 seconds per name) and reports `host_unresolved` for a name without an address. `POST /api/access/resolve {host}` resolves at once (when a name is added and by the "Refresh" button; 422 `unresolved` when there is no address). `GET /api/access` and the check answer carry `hosts` (addresses, `checked_at`, `failing`, `expired`). Mockup: `docs/design/next/access-ddns.html`. Tests replace the resolver with `server.SetHostResolver`.

## 12e. Changes after the 0.1 review
- The agent certificate is renewed over the channel (`cert.renew` / `cert.renewed`) when less than a third of its lifetime is left;
  the agent re-reads the certificate before every connection. The reconnect pause is reset after a session longer than 30 seconds.
- The backup rollback of network and firewall changes (section 4.3, item 3) is implemented: the one-shot timer `linager-agent-change-rollback`
  runs `linager agent rollback-change` after 35 seconds and takes over the journal if the agent did not cope itself.
- apt operations are not tied to the stream: a lost connection does not stop `apt-get`, and a new stream attaches to the running operation and receives all events and the log from the start.
- The SSH lock-out warning is computed from the ports reported by `sshd -T` (the agent returns them in `firewall.state`).
- Server update: the verified binary is copied into a root-only directory (`/var/lib/linager-updater`) and its checksum is compared
  before installation; after a failure past the migrations the database is restored from the pre-update backup; downgrades are
  refused (server and agent); the stable channel does not accept versions with a suffix. New command `linager restore`.
- An agent rollout keeps its state in `settings` (`updates.rollout`) and continues after a server restart;
  a failed first agent in "first, then the rest" mode no longer stalls the rollout.
- `install.sh` started through a pipe (`curl … | sh`) gives the wizard the terminal back.
- Interface: the `ELSH 40` dot shape on large headings, the "Terminal" button on cards (address `#srv-<name>/<tab>`),
  agent update tags and banner, tabs of disabled modules are hidden.

## 13. Decisions and open questions

Decided by the owner:
- Agents connect directly to a separate port, not through a proxy. Behind Caddy, the public admin panel port is 443; the Linager admin panel port is set during installation (required, no default value).
- Platforms: Debian 12/13 and Ubuntu 22.04/24.04/26.04 LTS. A single installer for the apt/systemd family.
- Test environment: two machines on the owner's local network (access to be provided by the owner) plus containers in CI.
- The release signing key is created by the owner; secret `MINISIGN_KEY`.
- Frontend: Svelte 5 + Vite (my choice; this is only an internal web UI build tool).

Also decided:
- Admin panel modes in the wizard: (1) **Caddy** — the installer sets up Caddy on this same server, with a Let's Encrypt certificate for the specified domain, public port 443; (2) **Reverse proxy** — the installer installs and configures nothing, and only asks for the ports, the listen address, trusted proxies, and the base path (example configs in `docs/`).
- Ubuntu 22.04, 24.04, and 26.04 LTS. If `ufw` is active, the "Firewall" tab shows a warning; nothing else is done about it in 0.1.

Open:
1. Let's Encrypt requires the domain to point to the server and ports 80/443 to be reachable from the internet. This will not work on a closed network; the wizard will check availability and suggest the "Reverse proxy" mode. Test machines on the local network will verify the "Reverse proxy" and "Caddy" modes with a test certificate authority (staging) or without a certificate.
2. Access to the test machines via SSH key (see correspondence); passwords do not go into the repository or documents.
