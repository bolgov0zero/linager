# Updates, backups, and rollback

## How releases are built
Each GitHub release contains the binaries `linager_linux_amd64` and `linager_linux_arm64`, the files of the Windows agent (`linager-agent_windows_amd64.exe` and `.msi`), the application (`linager-desktop_…`), `SHA256SUMS`, and a signed `manifest.json` (version, channel, per-architecture binary checksums, number of DB migrations, minimum agent version, changelog). The minisign signature (Ed25519) is verified with the public key **embedded in the binary**. A binary that does not match the signed manifest is not installed, neither on the server nor on an agent.

## Updating the server
From the application: the "Updates" page (the header button with a counter), then "Update to X". From the terminal:

```bash
sudo linager update --check      # what is available
sudo linager update              # update (asks for confirmation; --yes to skip the prompt)
```

Order (visible step by step in the application): signature check → backup (`pg_dump` plus configuration and keys) → service stop → binary replacement (the previous one is kept in `/var/lib/linager-updater/prev/`, a root-only directory) → database migrations (forward only) → start and check (the application returns 200, the client port accepts connections, up to 60 s). **If the check fails, the previous binary is restored automatically.** Migrations cannot be rolled back, so a database copy is made before every update: `/var/backups/linager/linager-pre-update-*.tar.gz`. If the failure happens after the migrations have run, the **database is restored from that copy automatically** together with the binary: the previous version must not run on a newer schema. If the database cannot be restored, the error message contains the ready `linager restore …` command.

Before the service stops, the verified binary is copied into a root-only directory (`/var/lib/linager-updater/`), and its checksum is compared once more before installation: the release cache is writable by the server service and is not trusted after verification. **Downgrades are refused**: a signed but older release is installed neither from the application nor by `linager update`. If you really need one: `sudo linager update --version vX.Y.Z --allow-downgrade` (the database is not migrated back).

When started from the application, the update is run by the `linager-updater` service (a systemd path unit watches the request file and runs `linager update --apply-request` as root). The server service itself runs without root privileges and cannot replace the binary. The installer creates these units.

## Updating agents
Agents take the release **from the server** (client port, mTLS), so the server must be updated first. On the "Updates" page you can update one agent, selected agents, or all agents. Modes: all at once, one by one, or first one and then the rest; option "stop on error". An offline agent is queued and updated when it connects. A rollout survives a server restart: the queue continues where it stopped. In "first one, then the rest" mode a failure of the first agent stops the rollout when "stop on error" is on; otherwise the remaining agents are updated anyway. An agent refuses to install a version older than its own. An agent older than the minimum supported version is marked "not supported".

The agent verifies the signature and checksum, keeps the previous binary (`/var/lib/linager-agent/linager.prev`), replaces `/usr/local/bin/linager`, starts a **rollback timer** (`systemd-run`, runs the previous binary), and restarts. The new version confirms itself with its first successful connection to the server and cancels the timer. **If there is no connection, the previous binary is restored automatically after about 70 s.** Manually on the client host: `sudo linager agent update`, `linager agent status`.

### Stages of an agent update
The progress is shown as a list of stages: on the server page (in a frame under the header) and on the Updates page, in the row of the agent (the "Stages" button). Every stage has a mark, a time and details:

| Stage | What it shows |
|---|---|
| Download | the size (`7.2 MB of 18.4 MB`) |
| Signature and checksum | |
| Backup | |
| Replace the file, arm the rollback | the rollback countdown starts here |
| Restart the service | |
| Agent offline | how many seconds the agent has been gone and how long is left before the rollback |
| New version connected | the version the agent came back with |
| Confirmation | the new version cancelled the rollback timer |

The first five stages are reported by the agent itself; the server sees the last three: the connection dropping, the hello of the new version and the "confirmed" event. So the restart is no longer a black box. When the agent comes back with its old version, the "connected" stage is marked as failed, and an error at any stage shows where and why it stopped (the error text under the stage). The list is kept for the current run only: it disappears after a success and stays after an error or a rollback until the next run. An agent older than this version does not report the beginning and the end of its stages: its list is built from its events, and the last three stages are fully visible.

### Time limit and the journal of an update
The agent has **10 minutes** for the whole update (the download of the release, the check, the backup, the replacement); other commands are limited to two. The server waits 2 minutes longer. When the download does not fit, the "Download" stage ends with the error `context deadline exceeded` and the time it took to break.

To look into a failed update:
- **The journal of the agent** (`journalctl -u linager-agent`) has `update: …` lines for every stage: the start with the versions and the architecture, the answer of the server to the request of the binary and how long it took, every 10 seconds the progress of the download (bytes, percent, the average rate and the rate of the interval; `idle_s` is how many seconds no byte has arrived), the end of a stage and how long it was, and on an error the stage, the reason and `timed_out=true` when time ran out. The last line of a successful update is `update: installed, the rollback timer is armed`, after the restart `update confirmed`.
- **The journal of the server** (`journalctl -u linager`) has `agent update: …` lines: the command and how long it waits, the stages the agent reported, a dropped connection during the update (with the step and the time since the last message), the agent coming back on the new or on the old version, the confirmation, an error or a rollback. The `release file: sent` lines show how many bytes were really given to the agent, in how many milliseconds and at what rate; `release file: the transfer did not finish` says the agent broke off the download.

## Channels and release source
The `stable` channel (tags `vX.Y.Z`) or `beta` channel (`vX.Y.Z-beta.N`) is selected in the application. In `/etc/linager/server.yaml`:

```yaml
update:
  source: ""            # empty = GitHub bolgov0zero/linager; "owner/repo"; or a mirror URL serving releases.json
  channel: stable
```

Mirror (for closed networks): a directory with the release files and a `releases.json` in the GitHub API format (`tag_name`, `prerelease`, `body`, `assets[].name/browser_download_url`), served over HTTP(S).

## Backups
```bash
sudo linager backup --keep 7     # /var/backups/linager/linager-backup-<date>.tar.gz (permissions 600)
```
The archive contains: `db.dump` (pg_dump, custom format), `config/server.yaml`, and `data/` (CA keys, the command-signing certificate and key). **The archive contains secrets; store it as carefully as the server itself.** The installer enables a daily backup at 02:00 (`linager-backup.timer`; the 7 most recent are kept).

Restore:
```bash
sudo linager restore /var/backups/linager/linager-backup-XXXX.tar.gz              # database only
sudo linager restore /var/backups/linager/linager-backup-XXXX.tar.gz --with-keys  # database and keys (moving to a new server)
```
The command stops the service, replaces the database contents with the dump from the archive **in a single transaction**
(on any error the database stays as it was), and starts the service again.
`--with-keys` puts the CA keys, the client-port certificate and the command-signing key back into the data directory:
this is needed when moving to a freshly installed server, so that clients keep connecting.
`server.yaml` from the archive is not restored (it holds the database password of the old installation); it stays in the archive for reference.

## Signing key (for the repository owner)
The key is generated once: `minisign -G -W -p minisign.pub -s minisign.key`. The contents of `minisign.key` go into the repository secret `MINISIGN_KEY` (plus `MINISIGN_PASSWORD` if it is password-protected). The line from `minisign.pub` is written into `internal/release/release.go` (`PublicKey`) and `install.sh` (`PUBKEY`). A release without the secret is published as a pre-release without a signature; the installer and the updater reject such releases.
