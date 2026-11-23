# Installing Linager

Supported: Debian 12/13 and Ubuntu 22.04/24.04/26.04 (amd64, systemd). The agent also runs on arm64, for example on a Raspberry Pi 4 with Raspberry Pi OS 64-bit (Debian 12/13). 32-bit systems are not supported, nor is the server on arm64.

## Quick install

```bash
curl -fsSL https://raw.githubusercontent.com/bolgov0zero/linager/main/install.sh | sh
```

Run the command as root, otherwise the installer stops with an error.

**After the install the Linager menu opens** (the same as the `linager` command without arguments shows, see [menu.md](menu.md)): when the wizard ends and prints its summary (the addresses, the administrator password), the script asks for Enter and opens the menu. The install by a link (below) does the same. The menu does not open when the install failed, when there is no terminal (automation) or when `--no-tui` is given.

## Safe option: download, verify, run

```bash
curl -fsSLO https://raw.githubusercontent.com/bolgov0zero/linager/main/install.sh
less install.sh                      # read the script
sudo sh install.sh
```

The script clears the screen, checks the system, downloads the release, verifies the checksum and the minisign signature (the key is embedded in the script), and starts the `linager install` wizard.

## Wizard

The first question: what to install, **Server** or **Client**.

### Server
Questions (the ports are required, there are no default values):

| Question | Description |
|---|---|
| Port for clients | clients connect here directly over TLS |
| Application port | the Linager application connects here over TLS ([connection](connection.md)) |
| Admin panel access | `Caddy on this server` or `Own reverse proxy` |
| Domain, HTTPS port | Caddy only: Let's Encrypt, requires open ports 80 and 443 and a DNS record |
| Admin panel port (HTTP) | own proxy only: your proxy connects here |
| Login and password | a password of at least 12 characters; leave empty to generate one and show it once |

Stages: system check, PostgreSQL (from the OS repository, localhost only, `scram-sha-256`), database and role with a random password, configuration `/etc/linager/server.yaml` (permissions 600, owner `linager`) and keys, administrator account, Caddy (if selected), `linager-server` service, startup check. The installation log is written to `/var/log/linager-install.log`.

In "Own reverse proxy" mode the wizard installs and configures nothing; it only asks for the ports. The admin panel listens on `0.0.0.0:<port>`, and `X-Forwarded-*` headers are accepted from any address (narrow the list in Settings → Admin panel access → Trusted proxies; `admin.trusted_proxies` in `server.yaml` is in force until something is saved there). The admin panel port is exposed directly, so restrict it with a firewall. Proxy examples: [proxy.md](proxy.md).

At the end, the wizard shows the admin panel address, the fingerprint of the client-port certificate, and a ready-to-use client installation command.

### Client
Questions: the server address `host:client-port` and confirmation of the fingerprint (or the `--fingerprint` flag). Then the wizard shows the **key** `XXXX-XXXX` and the **verification code**. Enter the key in the admin panel ("+ Server" → "Confirmations", the "Awaiting approval" block) and make sure the codes match. After confirmation, the `linager-agent` service starts. The address, port and fingerprint for this question are in the admin panel: "+ Server" → "Confirmations", the "Client connection details" block (the address is taken from the browser's address bar, the button copies `address:port`).

Two ways to skip the wizard and the key: install by a link and install over SSH (below).

### Install by a link
In "+ Server" → "Install by a link" make a link: its lifetime (15 minutes, 1 hour, 24 hours, 7 days), the number of servers (1, 5, 10, 25 or unlimited), and optionally a group and the allowed networks. The panel shows **two commands**: an external one (by the address that the server has on the internet) and a local one (by its address in your network). Run the one you need on the new server as root (without `sudo`: some systems do not have it; `wget` is needed, it is part of the base system of Debian and Ubuntu):

```bash
wget -nv -O- --no-check-certificate --pinnedpubkey 'sha256//…' 'https://panel.example.com:9443/v1/join/lk_…' | sh
```

The script finds out the OS and the architecture, takes the binary **from your server** (from its cache of signed releases, no GitHub), compares it with the signed manifest and runs `linager install agent --join-token …`. The server appears in the "Confirmations" tab marked "by a link": no key to type, just press "Confirm". When the link has "Approve servers automatically" on, the server is added at once. The agent reports its own name.

- The server in the command is checked by key pinning (`--pinnedpubkey`), so `--no-check-certificate` does not open a way to substitute it: only your server can answer.
- The token is shown **once**, when it is made. The panel keeps only its hash. A link can be revoked in the "Active links" block.
- The script can be read before it runs: the "Show script" button, or the same `wget` command without `| sh`.
- A link whose lifetime is over or whose uses are spent stops working by itself. The network limit checks the address from which the host talks to the server.
- The addresses of the commands come from `agents.hosts` in `server.yaml` (the names and addresses of the client-port certificate) and from the address you opened the panel with.
- By hand: `sudo linager install agent --server host:port --fingerprint SHA256:… --join-token lk_…` and `linager agent enroll … --join-token …`.

### Install over SSH
In "+ Server" → "Install over SSH" enter the address, port, login and password (or a private key) of the new server. The panel connects by itself and installs the agent; no confirmation is needed afterwards: the SSH sign-in has already shown that the server is yours.

1. **Check.** The panel shows the host key fingerprint (compare it with the console or the hosting panel: otherwise a password can be handed to a fake host), then checks the sign-in, the system (Debian 12/13, Ubuntu 22.04/24.04/26.04, amd64 or arm64), systemd and free disk space, the rights (root, `sudo` without a password or `sudo` with one: it is asked for and needed only while the install runs), **whether the host can reach the client port of this server** (with a choice of another server address) and whether an agent is already there (it can be reinstalled).
2. **Install.** A one-shot token (10 minutes, one server, auto-approve), the download of the agent from the server, the check against the signed manifest, `linager install agent`, pairing, the first connection of the agent. The output of the commands is in the "Output" block.
3. **Rollback.** If the agent does not connect within 60 seconds or the installer fails, the panel removes the agent from the host (`agent uninstall --purge`) and the server entry: the host stays as it was.

Security:
- The password, the key and the `sudo` password live only in the memory of the server while the check and the install run (at most 15 minutes without a call), and are written nowhere, the log and the audit included. The audit gets the address, the login and the result. A password is accepted over HTTPS only (except `localhost`).
- The panel refuses to connect to loopback, link-local (`169.254.0.0/16`, cloud metadata) and multicast addresses. The range can be narrowed in `server.yaml`: `ssh_install.allowed_networks: ["10.0.0.0/8", "192.168.0.0/16"]`.
- At most three open installs per administrator and ten checks in ten minutes.
- Hosts where the SSH sign-in needs a second factor (`keyboard-interactive` with a code) are not supported: use the install by a link for them.

## Unattended (automation)

```bash
sudo linager install server --no-tui --agent-port 9443 --app-port 9444 --admin-mode proxy --admin-port 8080 --admin-user admin
sudo linager install server --no-tui --agent-port 9443 --app-port 9444 --admin-mode caddy --domain linager.example.com
sudo linager install agent  --no-tui --server 10.0.0.5:9443 --fingerprint SHA256:...
```

Without a terminal, or with `--no-tui`, no questions are asked: all required parameters are taken from flags. `--lang ru|en` selects the language, `--force` allows a repeated installation, and `--admin-password-file` sets the password from a file.

## Removing a client
In the admin panel: open the server card and click "Delete server". You can remove the agent from the host at the same time. Manually on the host: `sudo linager agent uninstall --purge`.
