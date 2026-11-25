# The Linager application

The desktop application (macOS, Windows, Linux) shows the interface of a server in native windows. It replaced the web panel (removed in 0.10.0-beta.17): it is the only interface of the server. The server is installed with the terminal installer (`linager install server`); the application connects to a ready server.

**State (stage 2).** There is the server page: Overview, Metrics, Terminal, Services, Processes, Logs, Updates, Disk, Files (tree and table), Network, Firewall, Security, Users, Docker; dangerous actions ask first. Every tab can be changed: users, SSH sessions, fail2ban and keys, the network and the firewall (with a 30 second rollback), Docker (containers, images, editing a container). Windows of their own: the terminal (user list, snippets, Files, Manager), the note, files, the manager, the file editor. The Console: the list of servers, tabs of sessions, terminals, the Assistant, RDP and Keys (the key button of the top bar, the file is opened and saved through the dialogs of the system). The Windows tabs (users, Windows Update, programs, network adapters). Earlier: the connection with a certificate check, sign-in (login, password, 2FA), the dashboard in three layouts (cards, compact cards, rows), light and dark themes, Russian and English, a terminal in a window of its own. Self-update from a button in Settings → Updates. The tray: an icon in the notification area (below). Not yet: notifications, signed and notarized builds, sign-in with a passkey (removed).

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
| macOS (Apple Silicon only) | `linager-desktop_macos_arm64.zip` | `Linager.app` |
| Windows 10/11 x64 | `linager-desktop_windows_x64.msi` (installer), `linager-desktop-autoupdate_windows_x64.zip` (the archive that the self-update uses; it also works as a portable copy) | `linager-app.exe` (WebView2 is needed, Windows 11 has it) |
| Linux x64 | `linager-desktop_linux_x64.tar.gz` | `linager-app` (`libgtk-4` and `libwebkitgtk-6.0` are needed) |

Unpack and run. The builds are not signed by a developer or notarized: on macOS open the application from the context menu the first time (right click, "Open"); if macOS says the file is damaged (it does with a zip downloaded from the network), remove the quarantine: `xattr -dr com.apple.quarantine Linager.app`. Windows shows SmartScreen. On Windows there is also an installer, `linager-desktop_windows_x64.msi`: it installs for the current user only (no administrator rights) into `%LocalAppData%\Programs\Linager`, adds a Start menu shortcut and an entry in "Apps", and self-update works from there. On Linux the archive holds `linager.desktop` and `linager.png` for the menu of the desktop (the commands are in `README.txt` of the archive). Put the application where you can write (on macOS `~/Applications`, or `/Applications` as an administrator): self-update replaces the files in place.

The checksums of every file are in `SHA256SUMS` and in the signed `manifest.json`, with the same release signature (minisign) as the server and the agent.

## Updating the application

**Compact cards.** At the bottom of a card, instead of the address, there are small chips with numbers when there is something to look at: failed services (red), a reboot is needed (amber), system updates and updates of Docker images (blue, and amber from 10 packages or 5 images). At most three chips are shown, the rest fold into "+N" (the tooltip lists them). A server that does not answer shows "offline" and the updates it knew of in grey. Nothing to show: the address is there.

The gear of the top bar opens the **Settings** in a window of their own. Sections: "Application" (what is described below), "Profile" (password, 2FA, recovery codes), "Snippets", "Folders" (the tree of the folders of servers, see below), "Tags" (the registry of tags: rename, pick one of 16 colours, delete, make a new one; each shows how many servers carry it and how many users were given access by it), "Users" (roles, the servers, folders, tags and modules of an operator) "Server access" (the white or black list of addresses that may connect to the application port, a test of an address, the journal of the connections of the application; it is apart from the access of the application, and until it is saved the port follows that one; host names (DDNS) are resolved through DNS once a minute, like in the application), "Connection" (the ports of the agents and of the application) "Updates" (of the server and the agents: the check, the update, the cache of releases, the history) and "Journal" (the audit log). An operator and an auditor see only "Application" and "Profile". All the sections of the settings of the panel are moved. **Add server** (the button of the dashboard): confirmations (the agents that ask to join, and the data to connect by hand), install by a link and install over SSH. The edit mode of the dashboard (the button "Edit"): the order of the cards (drag or arrows), the group of a server, take off the dashboard (five seconds to undo; it comes back in "Add server → Confirmations") and delete a server. The window of the application cannot be made smaller than 1340 by 851. **Settings → Application**: the version, the channel (chosen: "Beta" offers every release, "Stable" only the finished ones; the channel of the running version by default), the buttons "Check" and "Update and restart". The application checks at the start and every six hours (can be turned off in the settings); the gear gets a dot when a new version exists. **An update is installed only on your button press.**

**Folders.** Settings → Folders is the tree of the folders of servers, up to 8 levels deep (the former groups became folders of the top level). A server lies in exactly one folder, at any level; the servers outside any folder are collected under "No folder". The rows are dragged: the line between rows changes the order, dropping onto a folder puts the row inside (a closed folder opens after a moment, the list scrolls at the edge of the window, Esc cancels the gesture); servers move the same way. There is the "⋯" menu and the right click (new subfolder, rename, "Move to", delete), a double click and F2 rename, Del deletes, Alt+↑/↓ changes the order. A name is unique inside its parent; a folder cannot be put into itself or into its own subfolder. **An access given on a folder covers all its servers and the folders under it, the future ones too**, so a move or a delete that changes who sees what shows who loses and who gains which servers and asks to confirm (the audit log has `group.move`, `group.delete`, `group.assign` with the number of users it touched). When a folder is deleted, its subfolders take its place, its servers go up one level, and the access given by that very folder is gone. Creating, renaming and moving can be undone by the button at the bottom. The folder filter of the dashboard includes its whole subtree, and the Console shows the tree of folders in its side list.

**The limit of GitHub.** The check reads the list of releases from the API of GitHub, which allows an address 60 requests an hour (an unchanged list is answered "not modified" and costs nothing). If several computers share one address and the limit is reached, the application says when it may ask again. An optional **GitHub token** (Settings → Application → Updates; a fine-grained token for public repositories without any permission is enough) moves the requests to the limit of the token, 5000 an hour; it is kept in the keychain of the system and sent only with the request of the list.

What the button does: the application reads the list of releases from GitHub, takes the newest one with a file for your system, verifies the signature of `manifest.json` and the version in it, downloads the file and compares its size and sha256 with the signed manifest, unpacks it next to itself and replaces itself (on macOS the whole `Linager.app` is swapped and the old copy is put back on a failure; on Windows the old file is renamed to `.old` and removed at the next start), then starts the new version and quits. A file that is unsigned or does not match the manifest is never installed. A build from a working tree (`make run`, version `dev`) and the run without a window (`-headless`) do not update.

Install the first beta with the application (`0.10.0-beta.4`) by hand; later updates arrive through the application. To build your own copy from the sources (Go, Node.js and, on macOS, the Xcode Command Line Tools are needed):

```bash
make -C app bundle   # app/bin/Linager.app
make -C app zip      # app/bin/linager-desktop_macos_arm64.zip (x86_64 on Intel), to hand it to another Mac
```

## The tray

The application lives in the notification area (the menu bar of macOS, the tray of Windows and Linux). The icon is the logo without a background: macOS colours it itself, on Windows and Linux it is dark or white to suit the panel. Closing the window **hides** the application in the tray instead of quitting; the Dock icon stays. Quit with "Quit" of the icon menu or with Cmd+Q. A click on the icon, with either button, opens its menu ("Open" brings the window back). A second start does not open a second copy, it shows the window of the first.

The menu of the icon: "Open" (the first item), a state line ("Online 12 of 15, attention: 2, waiting: 1": the thresholds of the dashboard; "waiting" is the agents that ask to be approved), the connections you chose, and "Quit" (the last item). When some servers have a problem or agents wait, a number stands next to the icon on macOS and a red dot sits on the icon on Windows and Linux.

**Connections in the tray.** On the server page, next to the "Note" button, there is a gear button: the settings of the server. Two blocks: "Tags" (labels of the server, kept on the Linager server in a common registry: up to 20 tags of 32 characters per server, up to 200 different ones; an administrator changes them; the change is in the audit log as `server.tags` with the number of users whose access it changed. A new name makes a tag in the registry; the tags that exist are suggested while typing. Names, colours and deleting of tags: Settings → Tags. A tag can give a user access: roles and access are in [security.md](security.md)) and "In the tray" (the connections terminal, RDP and Assistant, as many as you like, only those the server has). The chosen ones appear in the icon menu: the icon of the connection and the name of the server; a click opens the window with that connection. The list is kept in `tray.json` next to the profiles, apart for every Linager server.

**Start at login.** "Settings → Application → Start at login": the application starts when you sign in to the system, with its window hidden in the tray (macOS: a launch agent in `~/Library/LaunchAgents`, Windows: a `Run` key of the user in the registry, Linux: an autostart file in `~/.config/autostart`). After a self-update the new version waits for the old one to quit and only then starts.

On Linux the tray needs a desktop with StatusNotifier icons (KDE, GNOME with the AppIndicator extension). Without one the closed window cannot be brought back by the icon: start the application from the menu again and the window of the running copy is shown.

## Building and running

```bash
make -C app test       # tests of the interface and of the Go packages
make -C app run        # the window of the application
make -C app headless   # only the interface on a local address, no window
```

You need Go and Node.js. macOS: Xcode Command Line Tools. Windows: built without CGO, the machine needs WebView2 (Windows 11 has it). Linux: `libgtk-4-dev` and `libwebkitgtk-6.0-dev`. The build is checked in CI on all three systems (`.github/workflows/app.yml`).

## Limits of stage 1

- **Passkeys** were removed (0.10.0-beta.17): the application could not use them, the address of its window (`127.0.0.1`) differs from the address of the server. Use login, password and 2FA.
- **Linux:** WebKitGTK 6.0 is needed (Debian 13, Ubuntu 24.04 and newer). Debian 12 and Ubuntu 22.04 were not checked.
- **Signing:** the application is not signed or notarized (macOS asks to open it from the context menu, Windows shows SmartScreen).
