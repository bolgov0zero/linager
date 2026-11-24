# The Linager application

The desktop application (macOS, Windows, Linux) shows the interface of a server in native windows. It will replace the web panel: `web/` is frozen and will be removed after the transition. The server is installed with the terminal installer (`linager install server`); the application connects to a ready server.

**State (stage 2).** There is the server page: Overview, Metrics, Terminal, Services, Processes, Logs, Updates, Disk, Files (tree and table), Network, Firewall, Security, Users, Docker; dangerous actions ask first. Every tab can be changed: users, SSH sessions, fail2ban and keys, the network and the firewall (with a 30 second rollback), Docker (containers, images, editing a container). Windows of their own: the terminal (user list, snippets, Files, Manager), the note, files, the manager, the file editor. The Console: the list of servers, tabs of sessions, terminals, the Assistant, RDP and Keys (the key button of the top bar, the file is opened and saved through the dialogs of the system). The Windows tabs (users, Windows Update, programs, network adapters). Earlier: the connection with a certificate check, sign-in (login, password, 2FA), the dashboard in three layouts (cards, compact cards, rows), light and dark themes, Russian and English, a terminal in a window of its own. Self-update from a button in Settings → Updates. Not yet: other settings, the tray and notifications, signed and notarized builds, sign-in with a passkey.

## Connecting

1. The **application port** is on in the server's "Settings → Connection" (the installer asks for it). The certificate fingerprint is shown there too.
2. Enter `address:port` in the application. It connects and shows the fingerprint of the server certificate.
3. Compare it with the fingerprint in the server settings or in the installation summary and confirm. The application remembers it and accepts only this certificate from then on (the way SSH remembers a host key). If the certificate ever changes, the application refuses to connect and warns you.
4. Sign in. The password can be saved in the keychain of the system (Keychain on macOS, Credential Manager on Windows, Secret Service on Linux); then sign-in is automatic, and the 2FA code, if it is on, is still asked.

The session lasts seven days and is renewed by every request to the server (plain use of the dashboard counts): while the application is used there is no need to sign in again, and after seven days without activity the password and 2FA are asked again. The application keeps the session cookie in the keychain and forgets it on sign-out. A browser panel session stays at 12 hours as before. The server must be 0.10.0-beta.4 or newer.

The list of servers and the last login are in `<user config dir>/Linager/profiles.json` (mode 600, nothing secret: a fingerprint is public). `vault-last.json` remembers the path of the file chosen for the Keys for seven days, like the sign-in itself; the passphrase of the file is never kept. `prefs.json` lies next to it: the theme, the language, the layout of the dashboard, the sort of the rows and the section of the top bar (Dashboard or Console), so that they last between starts.

## How it works

Inside the application a local server listens on `127.0.0.1`. Every path on it starts with a random token (no other program or web page knows it), and a request with another `Host` is refused (protection against DNS rebinding). The windows open its address.

- The interface is served by the application itself.
- The API of the server is proxied over TLS 1.3 with the fingerprint pinned. It is checked on every connection. The session cookie is kept by the application and never reaches the page. SSE (live data) and WebSocket (the terminal) go through the same proxy.
- Its own calls (`/app/...`): profiles, the keychain, opening windows.

The interface (`app/frontend`, Svelte) does not depend on Wails, so it can also be opened in a browser: `make -C app headless` prints the address.

## Getting the application

The ready files are in the [releases](https://github.com/bolgov0zero/linager/releases) (from `0.10.0-beta.4`, betas for now):

| System | File | Inside |
|---|---|---|
| macOS (Apple Silicon only) | `Linager-macos-arm64.zip` | `Linager.app` |
| Windows 10/11 x64 | `Linager-windows-x64.zip` | `linager-app.exe` (WebView2 is needed, Windows 11 has it) |
| Linux x64 | `Linager-linux-x64.tar.gz` | `linager-app` (`libgtk-4` and `libwebkitgtk-6.0` are needed) |

Unpack and run. The builds are not signed by a developer or notarized: on macOS open the application from the context menu the first time (right click, "Open"); if macOS says the file is damaged (it does with a zip downloaded from the network), remove the quarantine: `xattr -dr com.apple.quarantine Linager.app`. Windows shows SmartScreen. Put the application where you can write (on macOS `~/Applications`, or `/Applications` as an administrator): self-update replaces the files in place.

The checksums of every file are in `SHA256SUMS` and in the signed `manifest.json`, with the same release signature (minisign) as the server and the agent.

## Updating the application

The gear of the top bar opens the **Settings** in a window of their own. Sections: "Application" (what is described below), "Profile" (password, 2FA, recovery codes, the list of passkeys: a new one cannot be added in the application), "Snippets", "Server groups", "Users" (roles, the servers and modules of an operator) "Access" (white and black lists of addresses, trusted proxies, the request log), "Connection" (the ports of the agents and of the application) "Updates" (of the server and the agents: the check, the update, the cache of releases, the history) and "Journal" (the audit log). An operator and an auditor see only "Application" and "Profile". All the sections of the settings of the panel are moved. **Add server** (the button of the dashboard): confirmations (the agents that ask to join, and the data to connect by hand), install by a link and install over SSH. The edit mode of the dashboard (the button "Edit"): the order of the cards (drag or arrows), the group of a server, take off the dashboard (five seconds to undo; it comes back in "Add server → Confirmations") and delete a server. The window of the application cannot be made smaller than 1340 by 851. **Settings → Application**: the version, the channel (a beta follows the betas, a stable version the stable ones), the buttons "Check" and "Update and restart". The application checks at the start and every six hours (can be turned off in the settings); the gear gets a dot when a new version exists. **An update is installed only on your button press.**

What the button does: the application reads the list of releases from GitHub, takes the newest one with a file for your system, verifies the signature of `manifest.json` and the version in it, downloads the file and compares its size and sha256 with the signed manifest, unpacks it next to itself and replaces itself (on macOS the whole `Linager.app` is swapped and the old copy is put back on a failure; on Windows the old file is renamed to `.old` and removed at the next start), then starts the new version and quits. A file that is unsigned or does not match the manifest is never installed. A build from a working tree (`make run`, version `dev`) and the run without a window (`-headless`) do not update.

Install the first beta with the application (`0.10.0-beta.4`) by hand; later updates arrive through the application. To build your own copy from the sources (Go, Node.js and, on macOS, the Xcode Command Line Tools are needed):

```bash
make -C app bundle   # app/bin/Linager.app
make -C app zip      # app/bin/Linager-macos-arm64.zip (x86_64 on Intel), to hand it to another Mac
```

## Building and running

```bash
make -C app test       # tests of the interface and of the Go packages
make -C app run        # the window of the application
make -C app headless   # only the interface on a local address, no window
```

You need Go and Node.js. macOS: Xcode Command Line Tools. Windows: built without CGO, the machine needs WebView2 (Windows 11 has it). Linux: `libgtk-4-dev` and `libwebkitgtk-6.0-dev`. The build is checked in CI on all three systems (`.github/workflows/app.yml`).

## Limits of stage 1

- **Passkeys** do not work in the application: the address of the window (`127.0.0.1`) differs from the address of the server, so WebAuthn refuses such a sign-in. Use login, password and 2FA. Passkeys need a separate solution.
- **Linux:** WebKitGTK 6.0 is needed (Debian 13, Ubuntu 24.04 and newer). Debian 12 and Ubuntu 22.04 were not checked.
- **Signing:** the application is not signed or notarized (macOS asks to open it from the context menu, Windows shows SmartScreen).
