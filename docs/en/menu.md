# The terminal menu

`linager` without arguments opens an interactive menu: a logo, the state of the host and numbered actions. The subcommands (`linager update`, `linager backup` and the rest) work as before; in scripts, from cron and in a pipe (when there is no terminal) `linager` still prints the help.

```bash
sudo linager            # the menu
sudo linager --lang ru  # the menu in Russian (the language is taken from LANG by default)
```

Without `sudo` the menu opens too, but the configuration, the database and the keys are closed to an ordinary user: some lines say "root needed", and the items that need rights start through `sudo` (they carry a `sudo` tag).

## How the mode is found

The menu looks at the disk, not at the server:

| Found | Shown |
|---|---|
| `/etc/linager/server.yaml` or `linager-server.service` | the server menu |
| `/var/lib/linager-agent/state.json` or `linager-agent.service` | the agent menu |
| both | a picker: two cards with the state, keys `1` and `2` |
| nothing | an offer to install the server or the agent |

The state is refreshed every 5 seconds. When something cannot be read (the database is down, for example), its line says so and the menu stays usable. An installation or a removal made from the menu changes the screen at once.

## Server menu

State: the service and its uptime, PostgreSQL, the application address, the client port, the number of clients and of those waiting for approval, the last backup, the access restriction, the version (and an available update, checked every 30 minutes).

| Item | What it does |
|---|---|
| 1 | `linager update` |
| 2 | `linager backup` |
| 3 | a list of archives (up to 9), a confirmation, `linager restore <archive> --yes` |
| 4 | the administrator password: `server create-admin` when there are no users yet, otherwise `server reset-password` |
| 5 | a confirmation, `linager access reset` |
| 6 | the server fingerprint, the client port and a ready command for the client |
| 7 | a confirmation, `systemctl restart linager-server` |
| 8 | `journalctl -u linager-server -f`; leave the logs with `Ctrl+C` |
| 0 | back (when the host has two modes) or exit |

## Agent menu

State: the service, the server address (or "not enrolled"), the enabled modules, the version.

| Item | What it does |
|---|---|
| 1 | asks the address and the fingerprint, `linager agent enroll`; when the agent is already enrolled, a confirmation first |
| 2 | `linager agent update` |
| 3 | a confirmation, `systemctl restart linager-agent` |
| 4 | `journalctl -u linager-agent -f` |
| 5 | a choice between "keep the keys" and "with the keys" (`--purge`), `linager agent uninstall` |

## How it works

- The items run the same subcommands as in a shell: the menu steps aside meanwhile, and the output stays on the screen until Enter.
- Dangerous actions (restore, turning off the access restriction, restarting a service) need a confirmation; `Enter` does not count, only `y` does.
- Server actions made from the menu (update, backup, restore, restart, creating the administrator) are written to the audit journal as "CLI" with `via: menu`. Turning off the access restriction and resetting a password write their own entry.
- The password of item 4 goes through a temporary file with permissions 600, removed right after the command.

## Resetting the administrator password

A new command that item 4 uses. It can also be called on its own:

```bash
echo 'new password' | sudo linager server reset-password --login admin --password-file -
```

The command sets the password, lifts a sign-in lock-out and ends all sessions of the user. Two-factor authentication stays on.
