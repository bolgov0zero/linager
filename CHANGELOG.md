# Changelog

## 0.10.0-beta.9

The result of the check of the application against the web panel (docs/design/app-parity.md): the missing functions are done. The application gets a tray. The Linager server gets the tags of a server (a migration, `GET`/`PUT /api/servers/{id}/tags`); the agent is unchanged.

- New (application): **roles**. An operator and an auditor no longer see what they cannot use: the buttons "Add server", "Edit", the deletion and the reboot of a server are for an administrator; the tabs of the server page follow the modules the agent reports and the modules given to an operator (an auditor has only the overview and the metrics); the terminal, RDP and Assistant buttons of the cards and the Console follow the modules too.
- New (application): **the tab "Management" of a Windows computer**: the service of the remote desktop (state, port, listening, sessions; turn it on and off, with a confirmation), the list of the sessions of the computer (open in the Assistant), the policy that lets the agent send Ctrl+Alt+Del, and the picture settings of the Assistant.
- New (application): **services**: ignore a failed service (the badge "ignore": it does not count in the status of the server), turn the autostart on and off, the filters All, Running, Stopped, Failed and the number of services.
- New (application): **package updates**: ignore and un-ignore a package.
- New (application): **processes**: the choice of how many to show (10, 20, 50, 100, all), the dialog of a termination with "forcibly (SIGKILL)" (on Windows "end the process tree"), and the note when the process is the Linager agent itself.
- Change (application): **rebooting a server asks to type its name**, like the deletion.
- New (application): **metrics**: the periods of 90, 180 and 360 days, the statistics min, average, p95 and peak under every chart, the marks of the events on the charts (yellow: service failure, red: reboot), the list "Events of the period", the table of the storage of the metrics and the sent traffic on the network chart. The overview shows swap, the boot time, the last contact and a note for an offline server.
- New (application): **dashboard**: the context menu of a server (terminal, remote desktop, Assistant, open the page, the group in the edit mode, delete), the tags "agent: update available" and "agent is outdated" for an administrator, and a mark on the gear when an update of the Linager server is available. Fix: the button "open the page" of a card was disabled.
- New (application): **snippets of the terminal** can be only typed, without the Enter that runs them (the chip next to the menu "Snippets"). The server page navigation shows the number of failed services and of containers with a new image.
- New (application): **the tray**. An icon in the menu bar / the tray (the logo without a background, in the colour of the panel) with the menu "Open", the state of the servers (online, with a problem, waiting agents), the connections you chose and "Quit". Closing the window hides the application in the tray (Cmd+Q and "Quit" end it); a second start shows the window of the first; the number of problems is next to the icon (a red dot on Windows and Linux).
- New (application): **settings of a server** (the gear next to "Note"): the tags of the server and the connections (terminal, RDP, Assistant) shown in the tray menu with the icon of the connection and the server name; a click opens the window with that connection. New (server): the tags are kept by the Linager server (`GET`/`PUT /api/servers/{id}/tags`, the migration `00021_server_tags`, the audit action `server.tags`; the server detail has `tags`).
- New (application): **Settings → Application → Start at login** (a launch agent on macOS, the `Run` key on Windows, an autostart file on Linux): the application starts hidden in the tray. After a self-update the new copy waits for the old one to quit (`-after <pid>`).
- Fix (application): **the Docker tab**: the tables of containers and images keep every row on one line with fixed columns; the published ports are listed once (the IPv4 and IPv6 pair was repeated), the first one is shown with "+N" and all of them in the tooltip; the status and the image are cut with an ellipsis. The buttons of the Console modes (terminal, RDP, Assistant) are lower.

## 0.10.0-beta.8

- Change (application): in the Console the buttons "Files" and "Manager" of a terminal are icons only. The RDP and Assistant tabs of the Console, the server page and their windows have the same two buttons (icons: Files, Manager) next to "In a separate window".
- Fix (application): the table of the agents in Settings → Updates keeps every row in one line: the system is shortened ("Debian 12"), the version and the status do not wrap, the progress of an update is a bar and one line ("3/8 · replace") and an error is cut with the full text in the tooltip, so a row no longer grows when an update starts or fails. The stages of a failed run open under the row on a click.
- New (application): **deleting a server** like in the web panel: the dialog asks to type the name of the server and offers "Also remove the agent from the server" (on by default; for an offline server it stays on and locked: the agent is removed when it connects). It is used from the server page and from the edit mode of the dashboard.
- New (application): **export of the metrics**: the button "Export CSV" of the Metrics tab saves the chosen period (a native "Save" dialog, the file is written by the application).
- New (application): **the banner "the agent is outdated"** on the server page for an administrator: the new version, "Update agent" with a confirmation, the progress and the stages of the update, the result and the rollback; it follows the updates of the page "Updates".
- New (application): **the update channel can be chosen** in Settings → Application (Beta or Stable); before, it was fixed by the running version (a beta followed the betas). The choice is remembered. Beta offers every release, Stable only finished ones; the application never goes back to an earlier version, and changing the channel drops the update that was offered.

## 0.10.0-beta.7

- New (application): **Add server**: the button "Add server" of the dashboard (with the number of agents that wait) opens a dialog with three tabs. **Confirmations**: the agents that ask to join (the code and the key to type, or a request that came by a link), the connected servers that are not on the dashboard yet, and the data to connect a client by hand (the address, the port and the fingerprint). **Install by a link**: a one-time link with a lifetime, the number of servers, a group, allowed networks and automatic approval, the commands for Linux and Windows (the local and the external one) and the text of the script, the list of links with their use and revoking. **Install over SSH**: the form (address, port, login, a password or a key), the check by stages, the questions to the person (the host key, the sudo password, the address the host reaches this server by, a reinstall), the install with its output and the open page of the new server. Passwords and keys stay in the memory of the dialog and of the server and are wiped at the end.
- New (application): **the edit mode of the dashboard**: the button "Edit" of the dashboard (becomes "Done") in all three layouts. In the mode every card has a bar: move before or after the neighbour (among the cards shown), the group of the server, take off the dashboard (five seconds to undo; the server comes back through "Add server → Confirmations") and delete the server (asks first); cards and the rows of the list can also be dragged. The order is saved by the server and is the same in the web panel. Clicking a card does not open the page while editing.
- Change (application): the window opens in, and cannot be made smaller than, the size of 1340 by 851 points: the interface is laid out for it.
- Not checked: a full install over SSH (there is no SSH host to try it on); only the form, the block of loopback addresses, the stages and the explanation of a failed check were tried.

## 0.10.0-beta.6

- New (application): the server page: the items RDP and Assistant of the left navigation have the button "In a separate window", like the terminal (when the tab is on the screen, the page goes to Overview, since a session can be open in one place only).
- New (application): **the Disk tab of the server page is complete** (it was a table before): a card per partition with a pie chart (used and free), the growth per day, the forecast "fills in N days", the inodes and the trend; "What takes the space" (a scan by the agent with the pie of categories and the partition switch); "The biggest directories" with drilling down and a path; "Cleanup" (journal, apt cache, unused Docker images and build cache, old kernels, and on Windows temporary files, Windows Update downloads, the Recycle Bin, the delivery cache, dumps and Windows.old) with the exact command shown before the confirmation.
- New (application): **the Keys remember the chosen file for seven days** (the same period as the sign-in; every use renews it): the window asks only for the passphrase, "Forget the file" drops it. Only the path is kept (in `vault-last.json`, mode 600, the page cannot write it), never the passphrase.
- New (application, Settings, part 1): **the gear of the top bar opens the Settings in a window of their own** (one window; the sections on the left). Sections of this part: **Application** (the version, the channel, the check and the update, the automatic check), **Profile** (password change, two-factor authentication with the secret and the code, the recovery codes shown once with copy and download, the list of passkeys with removal: a passkey cannot be added in the application), **Snippets** (add, edit, delete, order; the terminal menu "Snippets" has "Manage snippets…") and **Groups** (add, rename, order, delete). An operator and an auditor see only the application and their profile. With part 4 the sections of the web panel are all in the Settings.
- New (application, Settings, part 2): **Users** (add with a role, change the role and the access, set a password, reset 2FA, delete; the role Operator or Auditor gets the servers and groups and, for an operator, the modules; modules that give root access are marked and warned about) and **Access to the admin panel** (off, a white list or a black list of addresses, DDNS names that the server resolves, the switch "only through trusted proxies", the table of trusted proxies, a test of an address, the log of requests with filters and retention, "add to the list" from a log row; a change is checked by the server against your own address, asks for a confirmation and can be reverted).
- New (application, Settings, part 3): **Connection** (the certificate fingerprint, the port of the agents and the port of the application with the state of the listener; a change asks for a confirmation, shows the progress of agents moving to a new port and "close the old port"; the agent port cannot be changed while an agent that cannot follow is online) and **Journal** (the audit log: filters by result, group of actions, text and person, the details of a row, "show more", a refresh every 15 seconds).
- New (application, Settings, part 4): **Updates** of the Linager server and of the agents: the server (the version, the channel, the check, the release notes, "Update" with a confirmation and the progress of the steps; the dialog survives the restart of the service and shows the real result when the server is back), the cache of the downloaded releases (download ahead of time, delete the old ones), the agents (the table with the status of each, update one, the chosen or all, the order "all at once, one by one, the first and then the rest", "stop on error", the stages of a failed run, the countdown of the rollback), the history of updates and the commands of the terminal. With this part all the sections of the Settings of the web panel are in the application.
- Change (CI): the Linux libraries of the application are installed without the recommended packages (the step took 17 minutes).

## 0.10.0-beta.5

- Fix (application): the Console mode switch shows the icon and the count in one row; the Console search has a cross that clears it; the sections of the top bar are Console, Dashboard (in this order).
- Fix (application): the dashboard list and cards no longer wrap: the badges of the list have short forms ("reboot", "updates: 3"), the system name is shortened ("Debian 12", "Windows 10 Pro", the full name is in the tooltip), the uptime stays in one line, the card shows the address and the system in one line and the agent version under it.
- New (application): the cards, the compact cards and the list have buttons for the remote desktop (RDP) and the Assistant (on Windows, the Assistant also on a Linux computer with a graphical console); they open the session in a window of its own. The RDP and Assistant tabs of the Console have a button "In a separate window" (the tab closes: a session can be open in one place only).
- Change (application): in the Services tab and in the Manager the restart button comes before the stop button.

## 0.10.0-beta.4

Migration 00020 (`sessions.sliding_secs`). **Update the server** for the longer session.

- New (application): **the application is in the release files and updates itself.** The release carries `Linager-macos-arm64.zip` (Apple Silicon only, ad hoc signed, not notarized), `Linager-windows-x64.zip` and `Linager-linux-x64.tar.gz`; their checksums are in the signed `manifest.json` (keys `app_darwin_arm64`, `app_windows_amd64`, `app_linux_amd64`) and in `SHA256SUMS`. The gear of the top bar opens **Settings → Updates**: the version and the channel (a beta follows the betas), a check at the start and every six hours (can be turned off), and **"Update and restart"** on a button press, never by itself. The application reads the releases from GitHub, verifies the minisign signature of the manifest and the sha256 and size of the file, and only then replaces itself (the macOS bundle is swapped whole and rolled back on a failure; the Windows program is renamed and the old file removed at the next start) and starts again. A build from a working tree (version `dev`) and the run without a window do not update. The Windows and Linux swap was not tried on those systems yet. **Install this release by hand once; later updates come through the application.**
- New (application): **the server page**: the left navigation of the product (General, System, File system, Network and security, Applications; collapsible, the agent version below), the header with Reboot and Delete (a confirmation dialog first), a reboot banner, and the tabs Overview, Metrics, Terminal (also in a window of its own), Services (start, stop, restart; an important service asks again), Processes (terminate), Logs (journal stream), Updates and Disk. A click on a card or a row of the dashboard opens it. The tabs Files (folder tree and table), Network, Firewall, Security, Users and Docker are view-only for now (changes with the rollback timer stay in the web panel). The note, the files, the manager (processes and services) and the file editor open in windows of their own; the terminal window has the user list, the snippets, Files and Manager buttons.
- New (application): the Updates tab checks for updates and installs **the chosen packages** (checkboxes, "security only", progress and the apt output); the metrics charts show a tooltip on hover, with a switch to move it across all charts; the badges LIN and WIN have their own colours. Server and agent: the apt stream accepts `mode=packages&names=a,b` (**update the agent** for it).
- Fix (application): in every list only the data scrolls and the table header stays; the dashboard summary and toolbar stay in place; the Services list fits the window; the log no longer jumps to the end while it is read; the Files window got its missing styles; the Manager window has the common toolbar (like the terminal), terminate is red and restart is orange.
- Fix (application): the terminal and the file manager of the server page restarted on every refresh of the server record; the record is updated in place now. The Updates tab has no log block. Every checkbox is a custom one (like the selects). The note window renders Markdown (the preview opens first, a double click or the pencil edits, the text is saved by itself).
- New (application, stage 2c, part 1): **the Users tab can change users**: add (generated or typed password, groups, sudo, an SSH key, the commands shown before), properties (sudo, groups, shell, lock), a new password, ending the sessions, deleting with a typed name; the terminal opens as the chosen user. The Manager window has a tab "Users": who is logged in over SSH, and "End" for a session or for all sessions of a user. **Security** is editable too: the state with hints, ending sessions, fail2ban (install with a live output, settings, the list and the history of bans, a ban by hand, unban), a ban from the list of failed attempts, removing a key from `authorized_keys`. **Network** is editable: an interface (static or DHCP, address, gateway, MTU, up or down) goes into a draft checked while typing; applying asks again when the primary interface or the connection of the agent is touched, then the countdown banner (30 s) asks to confirm or to roll back (the agent rolls back by itself); the DNS servers and search domains; the configuration as text (ifupdown, or a netplan file). **Firewall** is editable: rules of the three chains (columns or as nft text, policy, order, delete, the journal switch), a rule by a form or as nft text, the address lists (sets), the listening ports with "close" and "allow only from", the journal of the firewall (period, filters, grouping, a block or a list from an event, CSV); the changes are a draft saved by itself, checked by the agent with nft, applied with the same warnings and the 30 s countdown with a rollback. **Docker** is editable: containers (start, stop, restart, logs, remove, compose projects with their own update, restart and logs), the check of images for updates and the update of the marked ones, the update of one container with a live output, the images (remove, pull), ignoring the updates of a container, and the dialog "Edit container" (name, image, restart policy, command, ports, mounts, variables, limits; the review shows what is changed on the spot and what recreates the container). Every tab of the server page is editable now; what is left of stage 2: the Windows tabs.
- Fix (application): **files are saved to the disk**: the window of the application did not handle a download link; the application asks where to save (a native dialog) and writes the file itself (`/app/download`; only the downloads of the file manager). The search fields have a cross that clears them (Escape too). The filter chips have the same corners as the other controls.
- New (application, Windows tabs): the server page of a Windows computer has its own tabs: **Users** (local accounts with their groups, the last sign-in, who is signed in now; add, enable and disable, set a password, edit the groups, delete; end a session; every dangerous action asks first), **Updates** (Windows Update: check, only the security ones, all or the chosen, with the live output of the install and the reboot warning), **Programs** (installed applications, search and sort), **Network** (adapters: DHCP or a static address, gateway, DNS; a change waits for the confirmation for 30 seconds, the agent undoes it by itself). Services, Processes, Files, Disk and the PowerShell terminal are the same tabs as on Linux. With these the stages of the application are done except settings, the tray and notifications, self-update and signed builds.
- New (application, stage 3d): **Keys** (the personal encrypted vault of logins and passwords, the format of the web panel: Argon2id and AES-256-GCM, the file `.lvault` stays with the person, nothing on the server): the key button of the top bar opens the window (create a vault with a passphrase, open a file, list, search, add, change, delete, binding to groups and servers, a generated password, the auto-lock, a question on closing with unsaved changes); the file is opened and saved through the native dialogs of the system and written readable by its owner only; the decrypted keys live in the memory of the window only. The RDP form has the list "Key" (a chosen key signs in at once), the Assistant has a key button on the sign-in or UAC screen (type the login, the password or both). The stage 3 is done; what is left of the application: the Windows tabs.
- New (application, stage 3c): **the remote desktop (RDP)** of a Windows computer in a Console tab (mode "RDP") and on the server page (a tab "RDP", hidden while RDP is off or the edition has none): the sign-in form (the login is remembered, the password never; an offer to turn the service on), the client IronRDP (WebAssembly, loaded when first needed) over the application's proxy and the agent, the keyboard (Cmd of a Mac works as Ctrl, a Cmd tapped alone is the Windows key, the clipboard shortcuts carry the clipboard), Ctrl+Alt+Del, Task Manager, Win, Win+L, Win+R, Alt+Tab, the clipboard both ways, the scale (fit or 100%), a change of the resolution with the window, full screen. The window of the application grew by the client (about 6 MB). Filling the form from a key comes with the Keys (part 3d).
- New (application, stage 3b): **the Assistant**: the screen of a Windows session, or of a Linux computer with a graphical console, in a Console tab (mode "Assistant") and on the server page (a tab "Assistant"): the picture as JPEG tiles on a canvas, the mouse, the keyboard by scan codes (Cmd of a Mac works as Ctrl, a Cmd tapped alone is the Windows key), the wheel, the clipboard both ways, the session list, the display (fit, fill, 100%, full screen), Ctrl+Alt+Del, Task Manager, Win, Win+L, Win+R, Alt+Tab, the monitors, the line under the screen (resolution, colours, frames, traffic), the dialog "Quality and frame rate" (presets, colours, frame rate, traffic limit, the pointer), a warning on the sign-in or UAC screen, a reconnect after a sign-in. A hidden tab pauses the pictures. One session can be open in one place only (the second place gets a note). Typing a key on the sign-in or UAC screen comes with the Keys (part 3d); the Dispatcher of Windows with the Windows tabs.
- New (application, stage 3a): **the Console**: a side list of servers by groups (fold, search, the mode switch Terminal, RDP and Assistant with the numbers, the filters by the system and the state), tabs of the open sessions (one tab per server and mode, scrolling arrows), a right click on a server: Files, Manager, the server page. The terminals live on in the background while another section is on the screen; the tabs, the mode, the folded groups and the filters are kept and come back after a restart (a restored tab connects when it is first opened). RDP and the Assistant are the next parts of stage 3.
- Fix (application): **the choices are remembered between starts**: the theme, the language, the layout of the dashboard (cards, compact cards, rows), the sort of the rows and the section of the top bar (Dashboard or Console). The window of the application is served from a random address, so the storage of the web view was lost at every start; the choices are kept in `prefs.json` next to the profiles file now. The Console section shows a stub until its stage.
- Change (application): the file manager opens the folder `/root` (the agent works as root; the drive `C:` on Windows; `/` when there is none; the tree shows the folders on the way, the files window can be opened at a given folder), the context menu closes on any click, Escape and when the pointer leaves it; a click on a header of the dashboard "Rows" list sorts by the column (again: reversed, third time: the server order); the topbar shows the server name as a green badge.
- New (application): the file manager has Up (with its arrow), New folder, New file and Upload (also by dropping files), the selection (Ctrl/Cmd and Shift), and the context menu of the web panel: Open, Download, Download as zip, Cut, Copy, Paste, Paste into folder, Rename, Properties (rights, owner, size of a folder), Delete; a name that is taken asks to replace or keep both. The terminal button is just "User".
- New: **the session of the application lasts seven days and is renewed by every request.** A session made on the application port is kept for seven days of idle time; while the application is used it never ends, after seven days without a request the password and 2FA are asked again. A browser session keeps its fixed 12 hours. The application keeps the session cookie in the system keychain (next to the saved password), opens the last server at once on start and forgets the cookie on sign-out or when the server no longer knows it.

## 0.10.0-beta.3

- Fix: **the application could not sign in on a server with the access restriction "only through a proxy"**: the application port is a direct connection, and every request to it was refused with a plain 403. The rule does not apply to the application port any more; the white and black lists still judge the real address of the connection. **Update the server.**
- Fix (application): the 2FA step is a screen of its own (the code is reduced to digits, so "123 456" works, it signs in at the sixth digit, a recovery code can be used); a sign-in that succeeded is no longer reported as failed when the keychain cannot be written; an unexpected answer of the server is shown with its HTTP status.
- Change (application): sign-in with a passkey is not offered (the window of the application is not the site a passkey belongs to); it goes away with `web/`.

## 0.10.0-beta.2

The desktop application, stage 1. The server and the agent are unchanged: this release carries the sources and the docs of the application (`app/`), the application itself is not in the release files yet. No database migration.

- New: **the Linager application** (`app/`, Wails v3, a separate Go module): connects to the application port of a server over TLS 1.3 with the certificate fingerprint pinned (confirmed by the person on the first connection, like an SSH host key), signs in (login, password, 2FA; the password can be kept in the system keychain), shows the dashboard in three layouts (cards, compact cards, rows) with live data and opens a terminal in a native window of its own. The interface follows the new design mockup (`docs/design/redesign`), uses the current Linager logo, Medeo Sans and Medeo Mono, Lucide icons, Russian and English, light and dark themes.
- New: a local server inside the application (loopback, a random token in every path) that serves the interface, proxies the API (the session cookie never reaches the page) and offers profiles, the keychain and windows. The interface can also be opened in a browser (`make -C app headless`).
- Not yet: the server page, files, the editor, the Console, RDP and the Assistant, settings, the tray, self-update; passkey sign-in does not work in the application (see the docs).
- Change: `web/` is frozen; releases go to the beta channel until told otherwise.
- Docs: [the application](docs/en/app.md) / [приложение](docs/ru/app.md); CI builds the application on Linux, macOS and Windows (`.github/workflows/app.yml`).

## 0.10.0-beta.1

The application port and the section "Connection". Migration 00019 (the port an agent connected to). **Update the agents** to follow a change of the agent port (0.10.0-beta.1 or newer); older agents keep working, they just do not move.

- New: **the application port**, a third port of the server for the Linager application: TLS 1.3 with the certificate of the agent port, the same API as the admin panel. The reverse proxy headers are dropped on it, so a client cannot name its own address. The installation wizard asks for it (`--app-port`, no default), `server.yaml` gets `app.listen`. On a server installed earlier it is off until set in the panel.
- New: **Settings, "Connection"**: the fingerprint of the certificate and the two ports, the agent port and the application port, changed without a restart (a new port is opened before the old one is closed, a busy port leaves everything as it was). The values are kept in the database and win over `server.yaml`; a reinstall with `--force` takes the answers of the wizard.
- New: **a change of the agent port moves the agents by themselves.** The server keeps the old port open and sends every agent the new one (`server.move`, signed); an agent tries the new address first, goes back to the old one after three failed attempts and ignores that port for an hour. The old port closes when every agent has moved (progress "N of M" in the panel), or with the button "Close the old port". While an online agent older than 0.10.0-beta.1 exists the change is refused and the agents are listed. Audit: `connection.update`, `connection.close_old`.
- Change: the install dialogs, SSH install and install links use the current agent port.
- Docs: [connection](docs/en/connection.md) / [подключение](docs/ru/connection.md); the installation docs got `--app-port`.

## 0.9.1

Web interface, the audit log and the Windows agent. No database migration. **Update the Windows agents** for the "Programs" tab (an old agent shows the tab with a request to update it).

- Change: **the audit log speaks in words.** The actions that showed as raw keys (`disk.scan`, `disk.clean`, `docker.check`, `network.dns`, `join.create`, `join.revoke`, `group.reorder`, `server.note`, `user.access`, `assistant.type`, Fail2ban, SSH install, the Windows actions `win.*` and others) have a readable text in both languages. The text is chosen in the browser by the key, so the old records are readable too. A test compares every action the server writes with the list of known keys and the two languages, so a new action without a text does not pass.
- New: **filters in the Console**: a panel that slides up at the bottom of the side list, **OS** (all, Linux, Windows) and **Status** (all, online, offline). They apply to the list and to the numbers on the mode buttons and are remembered in the browser.
- Change: **dialogs are floating windows** (a computer; on a phone they stay sheets). No dimmed backdrop, the page behind stays alive; a dialog moves by its header and cannot be dragged out of the screen, a click on it brings it to the front, Escape closes only the front one, several dialogs can be open at once. Done once in `Modal.svelte`, so all of them changed together.
- Change: **the top bar has no "live · time" indicator** any more (the dot and the clock).
- New: **the "Programs" tab of a Windows computer** (the module `programs`, the operator right `programs`): installed programs, view only. The agent reads the Uninstall keys of the registry (the computer in the 64 and 32 bit views and the loaded profiles), leaves out system components and Windows updates and shows a program once; name, version, publisher, install date, size, for whom; search and sorting. Nothing is started or removed.
- New: **the Assistant display mode "Stretch to the whole window"**: the picture fills the window and takes its proportions (the resolution of the computer is not changed; the pointer is mapped per axis). The RDP form already had "Adjust the resolution to the window" (on by default), nothing changed there.

## 0.9.0

Keys: a personal encrypted file of logins and passwords for RDP and for the sign-in screen and UAC of the Assistant. No database migration. **Update the agents too:** the Windows agent types the text on the sign-in screen (an old agent shows the key button but answers that typing is not supported). The steps of it are the betas 0.9.0-beta.1 to 0.9.0-beta.4 below; the docs are `docs/en/keys.md`, `docs/ru/keys.md`.

- New: **Keys**, the item "Keys" in the menu of the top bar of every user. The file is made and opened in the browser (Argon2id and AES-256-GCM with an authenticated header, plain JavaScript), kept by the person and never stored on the server, in the database or in the audit log. It is opened once per tab, the decrypted keys live only in the memory of the page; a closed tab, a reload, a sign-out and the auto-lock forget them. A key has a name, a type (RDP, UAC or any), a binding to several groups and servers (it only sets the order in the lists), a login, a domain, a password and a note. While the vault is open the menu item has an open lock icon.
- New: **the list "Key" in the RDP form**: a floating list (this server's keys first, then its group's, then for all); choosing a key connects at once. With a closed vault "Open…" opens a small window with the file and the passphrase. The password stays in the browser and goes into the CredSSP of the client.
- New: **typing a key into the sign-in screen and into a UAC prompt** (the Assistant of a Windows computer): a key button appears on the bar while the screen is the secure desktop; each key has "Login" (the login and Tab), "Password" (the password and Enter) and "Sign in" (the login, Tab, the password, Enter). The text is typed as characters (`SendInput` Unicode, no clipboard) and only while the active desktop is Winlogon, checked at the moment of typing. The audit log has `assistant.type` (the name of the key and the number of characters, never the text). The password passes through the panel server in memory; there is no "browser to agent" encryption yet.
- Fix: **the Assistant ended with "Connection closed" after a sign-in.** It connects again by itself now: a connection that ends within 30 seconds after the screen was the sign-in (or after a key was typed into it) is made again silently, patiently (up to twelve tries in a minute), and the last picture stays until the new one comes.

## 0.9.0-beta.4

The web interface and the docs only.

- Change: **in the key list of the Assistant "Login" presses Tab after the login and "Password" presses Enter after the password** (they only typed the text before). "Password" is for a sign-in screen where the login is already there: it types the password and signs in; "Sign in" is still the login, Tab, the password, Enter. The buttons have tooltips that say it.

## 0.9.0-beta.3

Update the agents too (the Windows agent types the text), and the panel.

- New: **typing a key into the sign-in screen and into a UAC prompt** (the Assistant of a Windows computer; the docs `docs/en/keys.md`, `docs/ru/keys.md`). While the screen is the secure desktop (the lock screen, the sign-in, a UAC prompt; the badge "UAC"), the bar of the Assistant has a key button with the list of the keys of the type UAC and Any; each key has "Login", "Password" and "Sign in" (the login, Tab, the password, Enter). The text is typed as characters (`SendInput` with `KEYEVENTF_UNICODE`): the layout does not matter and the clipboard is not used. A new frame `Type` (22) and its answer `Typed` (7) in `internal/assist`; the helper types only while the active desktop is Winlogon and checks it at the moment of typing on the thread that sends the input, so a prompt that is gone does not get a password typed into the window of the person ("Typing refused"). The password passes through the panel server in memory (no "browser to agent" encryption yet). The audit log has `assistant.type` (the session, the name of the key, the number of characters, what is pressed after; never the text) and `typed` in `assistant.close`; at most 60 typings per opening. The rights are those of the Assistant. Windows only (a Linux computer answers "not supported"). NOT tried on a real Windows yet: the typing on the Winlogon desktop.
- Change: `KeyPicker` has a second look, a key button of a toolbar whose list has the buttons of what to type.

## 0.9.0-beta.2

The web interface and the docs only.

- Fix: **the list "Key" of the RDP form is a floating list.** It was opened inside the form and made the window taller; it is now moved to the page like the other drop-down lists (`Select`: on a phone a sheet from the bottom, closes on a click outside, on scroll and on Escape).
- Fix: **the first line of the list was "For <the id of the server>"** (a long UUID). It shows the name of the server now ("For this server" when the name is not known).
- Change: **"Open…" in the list of the RDP form (the vault is closed) opens a small window** with the file and the passphrase, not the whole window "Keys"; when the vault is open the list of keys unfolds by itself (`KeyOpenForm`, `KeyUnlockModal`; the window "Keys" uses the same form).
- Change: **an open lock in the menu.** While the vault is open, the item "Keys" of the menu has an open lock icon of the dark orange colour (`--os-linux`); in the "More" sheet of a phone the key icon becomes the lock. Icon `unlock` (Pixelarticons 2.4.2).
- Change: **choosing a key in the RDP form connects at once** (the login, the domain and the password go to the client and the connection starts). If it fails, the form comes back with the fields filled.

## 0.9.0-beta.1

Beta channel. The web interface and the docs only: no database migration, no change in the server or the agents.

- New: **Keys, a personal encrypted file of logins and passwords** (the item "Keys" in the menu of the top bar, for every user; the docs `docs/en/keys.md`, `docs/ru/keys.md`, the mockup `docs/design/next/keys.html`). The file is kept by the person, not by the panel: nothing is stored in the database, in the config or in the audit. It is made and opened in the browser: Argon2id (64 MiB, 3 passes) and AES-256-GCM with an authenticated header (`web/src/lib/keys/vault.ts`, `@noble/hashes` and `@noble/ciphers`: plain JavaScript, works over HTTP, no WebAssembly, no change in the CSP). The file is opened once per tab and the decrypted keys live only in the memory of the page (not in `localStorage`, `sessionStorage` or IndexedDB): a closed tab, a reload, a sign-out and the auto-lock (5, 15 or 30 minutes, or off) forget them; with unsaved changes the vault stays open until the file is saved and the browser asks before the tab is closed. A key has a name, a type (RDP, UAC or any), a binding to several groups and servers (it only sets the order in the lists), a login, a domain, a password and a note.
- New: **the list "Key" in the RDP form** (the tab and the floating window) fills the login, the domain and the password from the vault; the keys of this server come first, then those of its group, then those for all, the rest are behind "Show all". The password stays in the browser and goes into the CredSSP of the client, as before.
- Change: the picker of servers and groups of the access of an operator (`AccessServersModal`) takes its own captions, so the binding of a key uses the same window. Icons `key`, `eye` and `eye-off` (Pixelarticons 2.4.2).
- Not in this beta: typing a key into the UAC prompt and the Windows sign-in screen (the next step: a key button in the Assistant). Keys of the type UAC are kept already.

## 0.8.6

Small changes in the panel only. No database migration, no change in the agents.

- Change: **the badge of the Assistant is called "UAC"** (it was "Lock / sign-in / UAC screen", in Russian "Экран блокировки / входа / UAC"). It shows in the header of the window, in the tab and in the line under the screen when the computer shows the lock, sign-in or UAC screen.
- Change: **the line under the screen of the Assistant (resolution, colours, frames, traffic) is hidden by default.** It is turned on with the item "Information" of the "More" menu; a choice made before stays as it was (`linager.assist.info`).
- Change: **the page has a smaller padding** (`14px 14px` instead of `22px 14px`), so the space under the dashboard is less.

## 0.8.5

Documentation, the landing and the screenshots only. No database migration, no change in the panel or the agents.

- Change: **the README (ru, en) and the landing tell about the Console view, the Assistant (Windows and Linux), RDP and the Dispatcher.** New rows in the table of features, a new section of screenshots, two new blocks and three new cards on the landing, the slider of the interface has the Console and the Assistant, and the notes on what the Assistant for Linux needs (GNOME on Wayland) and RDP (Pro, Enterprise or Server).
- Change: **all screenshots are taken again** (`docs/screenshots/ru` and `en`): the top bar, the Pixelarticons icons and the buttons of 0.7-0.8 were not in the old ones. New shots: `console`, `assistant`, `assistant-linux`, `assistant-manage`, `dispatcher`, `rdp`. The screens of the Assistant are a picture drawn by the demo simulator.
- New: **the page "The Console view"** (`docs/en/console.md`, `docs/ru/console.md`, in the menu of the site): the side list, the modes, the tabs and the floating windows, the rights.
- Fix: the Russian docs of the Windows agent call the tab "RDP" (it was "Рабочий стол" in two places since 0.8.3).
- Dev: the simulator `linager-sim` gives the Linux host "monitoring" a graphical console (`Gui`) and the drawn Assistant stream, so that the Assistant for Linux can be shot; not part of the released binary.

## 0.8.4

- Change: **the buttons of a card are of one colour.** The button "Assistant" in the card of a Windows or Linux server is no longer set apart by the accent colour: it looks like the Terminal, Open and Delete buttons.
- Change: **the badge of Linux is orange** (`--os-linux`; Windows stays blue), and the colours of the badges are the same in all the views: the cards, the grid and the Console (where the short "LIN" and "WIN" were grey before).
- Change: **the views of the dashboard are "Cards / Grid / Console"** (Russian: "Карточки / Сетка / Консоль"; they were "Map" and "Пульт"). Only the names changed, in the interface, the docs, the README and the landing.
- Fix: **the role "Administrator" is written with a capital letter** in the list of roles and in the badge of the list of users, like "Operator" and "Auditor".

## 0.8.3

- Change: **the tab "Desktop" of a Windows computer is called "RDP"** (the tab, the floating window, the item in the lists of windows and in the context menu of a tile, the right of an operator, the audit journal and the docs). The name says what it is, and it is no longer mixed up with the screen of the Assistant. Only the names changed.
- Change: **the list of the modules of an operator** (Settings, Access) is split into the groups of the sidebar (General, System, File system, Network and protection, Applications), and the name of the module "Desktop" is "RDP" there too. The description of the module "Assistant" says that it is not only for Windows any more (the Linux desktop too).

## 0.8.2

- Fix: **the list of the window "Files" in a narrow window.** The rights, the owner, the size and the date no longer wrap onto a second line; the columns drop out one by one as the list gets narrower (the owner, then the rights, then the date, then the size), and the name stays. An empty owner (Windows) is shown as "—".
- New: **the Assistant for Linux wakes a dark screen.** When the screen of the computer goes dark (the sign-in or the lock screen turns the monitor off), the helper no longer ends with "no display shows a picture now": it presses Shift on its virtual keyboard (`uinput`; the key does nothing on those screens), repeats it every two seconds and waits up to ten for the picture. Meanwhile the browser shows the plain "Connecting to the session…" stub with "the screen of the computer is off; switching it on" and no technical hint (a new frame `TypeWaking`, `{on: true|false}`, from the helper to the browser). The same works when the screen goes dark while the Assistant is open. If the screen stays dark (the monitor is switched off, the computer is asleep), the session ends with the reason. Update the agents too.

## 0.8.1

Small fixes of the interface. No database migration; the web interface only.

- Fix: **the floating window "Files".** The folder tree and the list scroll inside the window instead of going under its edge (the panes fill the window, `FilesTab` has `embedded`); the head of the list stays on top. A click on the title of a column sorts the list (name, rights, owner, size, date; a second click changes the direction). By default the list goes by name, and the folders are always on top.
- Fix: **the mode of a network card on Windows** is the project selector instead of the system one. The selectors (`Select`) are as tall as the text fields next to them; in "Processes" the number of rows is the size and the look of the buttons beside it (`size="chip"`).
- Fix: **the pencil in a note** is the same 24 px button as the cross; **the search field in "Users"** (Linux and Windows) is as tall as the buttons next to it.
- Fix: **the keys of the terminal.** The arrow icons are in the middle of their keys (also in the quick bar of a phone); with a mouse the keys are 29 px (24 px for the tabs of the modes), the touch size is kept on phones and touch screens.

## 0.8.0

The stable release of the **Assistant for Linux**: the console of a GNOME desktop on Wayland in the browser, and below the compositor the sign-in screen, the lock screen and X11. It gathers everything of 0.8.0-beta.1 to 0.8.0-beta.7 below. No database migration; update the server and the agents (the Linux Assistant needs the new agent).

- New: **the Assistant for Linux.** The screen of the desktop, the mouse, the keys, the text clipboard and the choice of a monitor, with no new session and no question to the person at the computer (GNOME Mutter over D-Bus, frames through `gst-launch-1.0 pipewiresrc`); the sign-in screen (GDM), the lock screen and X11 are read below the compositor (DRM/KMS in pure Go, input through `/dev/uinput`). The panel shows the tab only for computers with a graphical console, and switches by itself between the desktop and the lock or sign-in screen.
- Change: **no service messages when the Assistant switches between the desktop and the lock screen** (and back). The notices "connecting" and "closed" are not shown while the browser reconnects at the request of the computer; the last picture stays until the new one comes. The notice appears only when the reconnection gives up (after four tries in 40 seconds, or 30 seconds without a connection), and the button "Connect again" works as before.
- Change: **the text of the interface is not selectable where it is not needed.** Buttons, tabs, menus, headings, captions of fields, table heads and hints cannot be selected by an accidental drag any more; the values (the info about the system, tables, logs, the terminal, the editor) and every input field stay selectable (`app.css`: `user-select`).
- Change: **the toolbars of the view "Console" have one size of buttons.** The Assistant, the Desktop and the Terminal use the compact 24 px button (the size of "More"): the icon buttons "Files" and "Dispatcher" are 24 px with 16 px icons, the keys, copy, paste and close buttons of the Terminal and the buttons of the Desktop are no bigger than the others.
- The details of every step are in the entries of the betas below.

## 0.8.0-beta.7

- Fix: **a sign-out from the desktop ended the Assistant with "cannot read the screen: GNOME closed the remote session".** The Assistant for Linux connects again by itself now: the end frame of the helper has `reconnect` (`assist.End`), the browser connects again after a moment (a connection that cannot be opened is tried again for half a minute, not more than four times in 40 seconds) and the agent picks the way that suits the screen: the sign-in or the lock screen below the compositor, the desktop through GNOME. The helper below the compositor looks every two seconds whether the desktop is open (a person's own Wayland session, not locked) and asks for a connection again, so after the sign-in the desktop comes back with its clipboard.

## 0.8.0-beta.6

- Fix: **elements of the screen blinked with every action after the sign-in (the Assistant for Linux below the compositor).** The helper kept the mapped framebuffers by the number the kernel gave them; when the sign-in screen gave way to the desktop, the new compositor got the same numbers for other buffers, and the helper alternated between the new picture and an old buffer. The framebuffer is mapped for every picture and let go after it now.
- Note: below the compositor (the sign-in screen) the pointer of the computer is in the picture when the computer draws it itself, and the setting "show the pointer" cannot remove it; on the desktop the setting works.

## 0.8.0-beta.5

- Change: **the sign-in screen and the lock screen of GNOME: the reason is told.** The Assistant on the sign-in screen failed with "Session creation inhibited": gnome-shell inhibits remote access in every session mode that does not allow screen casts (the sign-in screen, the lock screen), it is a rule of GNOME, not a fault of the Assistant. The panel says so now and tells to sign in at the console; a session that GNOME closes (the screen was locked) is told the same way. Those screens are shown below the compositor, see the next entry.
- New (beta): **the Assistant for Linux below the compositor: the sign-in screen, the lock screen, X11.** The helper runs as root and reads the framebuffer of the display controller through DRM/KMS in pure Go (`GETRESOURCES`, `GETCRTC`, `GETFB2`, the buffer is mapped through its dma-buf, or as a dumb buffer when the driver exports none; XRGB8888, ARGB8888, XBGR8888, ABGR8888 and RGB565), and makes the mouse, the wheels and the keys as a device of the kernel through `/dev/uinput`. It is used for the session class `greeter` of logind (GDM before the sign-in), for a session whose lock hint is set and for X11 sessions; the picture is marked with the badge of the lock or sign-in screen. Only a linear framebuffer is read (virtual machines, some cards): a tiled one is refused with the reason. No clipboard, no pointer in the picture. The structs of the kernel interface and the numbers of the ioctls are checked by a test.
- Change: **the icons of the card buttons and of the buttons of the server header on a phone are 16 px** too (back, note, menu; terminal, Assistant, open, delete), as those of the server bar and of the side menu.

## 0.8.0-beta.4

- Change: **the menu of the top bar is a little larger again** (a row 31 px with 13 px text, 182 px wide: the 25 px of 0.7.4 were too small next to the buttons of the bar), and **the icons of the buttons of the server bar (note, reboot, delete) are 16 px**, as the icons of the side menu.
- Fix: **the sign-in screen of GDM: "the helper exited with code 1".** The sessions of the sign-in screen have names like `c1`, the helper took the number of the session as a number and refused `--session c1`. The helper gets a number now (0 when the session has none). When a helper stops with an error, the panel shows the end of what it wrote to its error output, not only its exit code.

## 0.8.0-beta.3

- Fix: **the sign-in screen of GDM: "the user of the session: unknown userid 60580".** The sign-in screen runs as a dynamic user of systemd that is not in `/etc/passwd`, so the agent (it has no libc to ask the system) could not find it. The helper takes the rights of such a user from the process that started the session (the group and the supplementary groups from `/proc/<leader>/status`, the name from logind), the home directory is `/tmp`. Still not tried: the bus and the compositor of the sign-in screen.

## 0.8.0-beta.2

- Fix: **the Assistant for Linux answered late.** The mouse and the keys went to GNOME one by one, each waiting for the answer of the compositor, so a fast movement of the mouse queued up the clicks behind it: the events go without waiting for an answer now. A change of the screen waited for the next tick of the loop (up to 200 ms when the screen was quiet): every new frame of the compositor wakes the loop now (`Frames()` of the host). The pipeline of GStreamer does not wait for its clock any more (`sync=false`).
- Fix: **rectangles of a slightly different shade on a flat dark area.** A tile sent while the traffic budget was spent had a lower JPEG quality than its neighbours sent later. Once the screen is still, the encoder sends again (8 tiles a picture) the tiles that were sent at a much lower quality, at the best one (`Encoder` keeps the quality of every tile). It helps the Windows Assistant too.
- Change: **the sign-in screen of GDM is a console too** (the session class `greeter` of logind, no user signed in yet): the tab of the Assistant stays while nobody has signed in, the helper starts as the user `gdm`. Not tried: it is the part most likely to need a change.
- Advice for a fast network: the preset "Normal" limits the traffic to 4 Mbit/s and lowers the quality by itself, which makes a moving window jerky; "Quality" or "No limit" in the picture settings is better on a local network.

## 0.8.0-beta.1

- New (beta): **the Assistant for Linux: the console of a GNOME desktop on Wayland.** The same view and control in the browser as on Windows, for a Linux computer where somebody is signed in to the desktop: the screen (JPEG tiles, quality and frame rate settings of the panel), the mouse, the keys, the text clipboard (Ctrl+C, Ctrl+V, the menu), the choice of the monitor. The agent (root) finds the active graphical session of seat0 in systemd-logind and starts a helper as the person of the session; the helper takes a stream of the monitor and a remote-desktop session from GNOME Mutter over D-Bus (`org.gnome.Mutter.ScreenCast` and `RemoteDesktop`, no dialog: a process of the person's own session may use them) and reads the frames with `gst-launch-1.0 pipewiresrc`. Needs GNOME on Wayland (Ubuntu 26.04 has no other session) and the packages `gstreamer1.0-tools` and `gstreamer1.0-pipewire`. The agent reports `gui` in its sample, and the panel offers the Assistant (the tab, the card button, the menu items, the Console mode) only for a computer that has a graphical console; Ctrl+Alt+Del, the Windows task manager, Win+R and "Management" are not offered for Linux. Not yet: X11, KDE, wlroots, the GDM sign-in screen and the lock screen, "All monitors", a native PipeWire client. **The server and the agent of the computer need this version.** Not tried on a real desktop except that the D-Bus calls and the frame of the stream were checked with a small program on Ubuntu 26 (GNOME Shell 50.1): the rest is for the beta.
- New: the module `assistant` is in the agent configuration of Linux (`config.CompleteAgent` adds it to `agent.yaml`); the dependency `github.com/godbus/dbus/v5` (pure Go).
- Dev: the shared constants of the helper moved to `agent/assist_types.go`; the pure parts of the Linux backend (logind, scan codes to evdev, the layout of the monitors, the clipboard formats) are tested on every platform.

## 0.7.4

- Change: **the menu of the top bar (theme, language, edit, settings, log out) is as small as the other menus**: a row is 25 px with 12 px text instead of 36 px with 14 px, the menu is 158 px wide instead of 198 px, the counter of the updates is a small mark.

## 0.7.3

- Fix: **the square icon buttons are square** (the cross of a dialog and of the window headers was 28x24, the up and down buttons of the groups 32x18 with a small icon off the centre). One rule now: an icon button is as wide as it is tall, 29 px (24 px for the compact one, 44 px on a phone), and carries the icon of the size Pixelarticons is drawn for (24 px in a 29 px button, 16 px in a 24 px one). It covers the buttons of the top bar, the cards, the server page, the Files and Dispatcher buttons, the move buttons of the groups, the snippets, the dashboard editor and the firewall rules.
- Fix: **the snippet and the user menus of a terminal window were taller than the buttons** (the list is a `.sel-list`, whose items have their own padding, and the buttons inside added theirs: 32 px instead of 24). An item is 24 px, like the items of the list of the groups and like the small buttons.

## 0.7.2

- Change: **the sizes follow the header of a floating window** (a 24 px button, 11 px text, a 36 px header), the only part that had the intended size. The header of a dialog is that header now (the title 14 px, the close button 24 px); a collapsed block ("Agents", "History", "Same from the terminal", the blocks of the settings and of the Metrics tab) is a thin bar of about 36 px instead of 64; the items of the dropdown menus (snippets, user, the menus of the buttons) are as tall as a small button (24 px, 11 px text); `.btn.sm` is one size everywhere (it was 33 px outside a window); the basic button and the field are 12 px text / 29 px instead of 13 px / 33 px, the top bar is 45 px instead of 57; the buttons of the server update stay in one row (a narrow panel shrinks them instead of moving the last one down). Phones keep the 44 px touch targets.

## 0.7.1

- Change: **the icons that were text characters are Pixelarticons too.** 0.7.0 replaced the drawn icons only; the buttons that used a character stay behind: minimize, maximize and close of the windows, the cross of the dialogs and of the rows and chips (Docker, Firewall sets, DNS, Fail2ban), the up and down buttons of the groups, the snippets and the firewall rules, the check mark of the chosen item in the menus (`ActionMenu` and `ContextMenu` got `checked`), the marks of the update and job stages, the arrows of the traffic on the cards, the Overview and Metrics, the tree and audit twists, the breadcrumbs, the arrows of the key panels, the move buttons of the dashboard editor and "to the end" of the logs. What stays text: "1920×1080", "×3", the "a → b" of versions and changes, and the legend of the traffic chart.

## 0.7.0

- Fix: **the buttons "Files" and "Dispatcher" were smaller than their neighbours** in the header of a window and on a tab: they were as tall as the icon. They are as tall as a button with words now, and nearly square (the remote desktop passes the size of the header, as the others do).
- Change: **all icons of the interface are the Pixelarticons set** (2.4.2, MIT, the license is in `web/src/lib/ui/pixelarticons-LICENSE.txt` and in `NOTICE`) instead of the hand-drawn 9x9 ones: the buttons, the tabs of a server page and the phone bar, the Console, the card buttons, the terminal keys, the dock of the windows, the marks of the security checks, the file icon, the pause and play of the logs, the menu. One place gives the paths (`ui/pixelart.ts`) and one maps the names (`ui/pixicons.ts`); the old inline drawings are gone. Pixelarticons is drawn on a 24x24 grid, so it is sharpest at 24 px (and 12 px on a 2x screen); the sizes that the buttons had (18, 16, 14, 13.5 px) are kept, so the icons are scaled there and a little softer. The icon for the Dispatcher is `analytics`, for the Assistant `mouse`. The logo and the picture of the home screen are not touched.

## 0.6.1

- New: **the buttons "Files" and "Dispatcher" on every connection.** Two icon buttons on the bar of the terminal, of the remote desktop (RDP) and of the Assistant, in a tab, in a floating window and in the Console. "Files" opens the "Files" tab in a floating window (the same as the item of the context menu of the Console); a button is there for whoever has the right for the module.
- New: **the Dispatcher works on Linux too** (the terminal has its button). Processes: the same list and sorting; "End" sends SIGTERM and "Force end" SIGKILL (the confirmation says so); services: systemd units with the filters, sorting, Start, Restart and Stop. The computer's system is read once when the window opens, and the texts follow it. A kernel thread such as `[kworker/0:1]` is shown whole.
- Dev: the buttons are one component (`assistant/ServerShortcuts.svelte`), the pixel icon `tasks` is new.


## 0.6.0

- New: **the Dispatcher in the Assistant.** An icon button on the bar of the Assistant (window, tab and the Console; shown to whoever has the right "Processes" or "Services") opens a floating window like the Task Manager, one per server, closed with the cross. **Processes:** refreshes every 3 s, sorted by any column (process, PID, user, CPU, RAM; the second click turns the order over), the totals of CPU and RAM in the headers, cells get warmer with the load, search by name, PID and user; "End" and "Force end", both with a confirmation and an audit entry. **Services:** the filters All / Running / Stopped / Failed, sorting by a column, "Restart" and "Stop" for a running service, "Start" for a stopped or failed one; protected services ask for confirmation as on the "Services" tab. A tab the person has no right for is not shown.
- Change (Windows agent): **"Force end" ends the process together with every process it started** (the tree, like `taskkill /F /T`; the deepest first, system processes and the agent are skipped; a parent that started later than its child is not taken for a parent, its PID was reused). "End" ends that process only. Windows has no softer way that a service can use for a window in the session of the person, so what is not saved is lost in both cases; the texts of the confirmation say so. **The Windows agent needs this version** for the tree; an older one ends only the process in both cases. Linux is not affected. The rules are in `dispatcher.ts` (sorting and actions, tested) and `agent/proctree.go` (tested).


## 0.5.4

- Fix: **the picture settings window of the Assistant blinked while it was moved** ("Loading..." and the form again). Every move gives the window a new object, and the form took that as a new computer and read the settings once more (an unsaved edit was lost too). The form follows the id of the computer only now.

## 0.5.3

- Fix: **a failed Windows service was counted on the dashboard ("1 service failed") but not marked in the "Services" list.** The counter takes the automatic services that stopped with an error, while the list showed every stopped service as "inactive". The list now marks the same services as "failed" (the filter "Failed" and the red row work as on Linux), both use one rule (`serviceFailed`, tested on Windows). **The Windows agent needs this version** (Updates page).

## 0.5.2

- Fix: **copy and paste from the keyboard in the remote desktop (RDP).** The client of the browser sent Cmd of a Mac to Windows as the Windows key (Cmd+C was Win+C), and it synchronised the clipboard only by itself in the background, which needs HTTPS and a permission that Safari does not give to a timer. Now Cmd works as Ctrl (a Cmd tapped alone is still the Windows key), Ctrl+V and Cmd+V send your clipboard to the computer first and then type the paste, and after Ctrl+C, Cmd+C and Cmd+X the clipboard of the computer is fetched into yours; a refusal of the browser is told once. The rules of the keys are in `desktop/keys.ts` (tested). Not tried on a real Windows computer.

## 0.5.1

- New: **the picture settings of the Assistant open in a floating window.** The item "Quality and frame rate…" no longer takes you to the "Management" tab (from a floating Assistant or the Console that left the page); it opens a small window with the same card (presets, colours, frame rate, traffic limit, "lower the quality by itself", the pointer), one per server, also in the Console (it is listed among the windows) and on the Assistant tab of a server page. A change takes effect at once, and the line under the screen shows the result. The card is one component (`AssistantQuality`) for the tab and the window; the presets and the traffic estimate moved to `quality.ts` (tested).
- Change: **the Assistant has no blue focus frame around the screen** any more (it covered 2 px of the remote picture's edge and showed after every mouse click).
- New: **the line under the screen can be hidden**: the item "Information" in the "More" menu (a tick means shown). One choice for every window and tab of the Assistant, remembered in the browser (`localStorage`, `linager.assist.info`); in the mode "Window fits the picture" the window refits at once.

## 0.5.0

The first stable release with the **Assistant** for Windows computers and the view **"Console" ("Пульт")**. It gathers everything of 0.4.0-beta.1 to 0.5.0-beta.2 below. One database migration (`00018`, the picture settings of the Assistant); the server updates as usual, **the Windows agents need this version for the Assistant** (the panel updates them from the "Updates" page; an older agent just does not offer the new tabs). Linux agents are not affected.

- New: **the Assistant**: the view and control of the console or of an RDP session of a Windows computer in the browser, like Aspia or Radmin. It is not a new RDP session (the person at the computer stays, Windows Home works), no password is asked: access follows the Linager rights (the operator module "Assistant", off by default; no consent prompt, every opening, closing and change is in the audit log). A tab "Assistant" and a window, a button on the cards of Windows servers, the session is chosen from a list. The picture is taken by a helper the agent starts in the session (GDI, JPEG tiles, adaptive quality under a traffic limit); the mouse and keys come back as scan codes. It follows the desktop Windows sends the input to, so the **lock screen, the sign-in screen and UAC prompts** are seen and controlled; **Ctrl+Alt+Del** goes to the console (a switch for the Windows policy "Software SAS generation" is in "Management"); the **text clipboard** works with Ctrl+V, Ctrl+C and Ctrl+X (on a Mac Cmd works as Ctrl) and from the menu; **several monitors** (one or all as one picture); an option to leave the pointer of the computer out of the picture (off by default, your own pointer has no lag); display modes (picture fits the window, window fits the picture, window fills the browser, picture at 100%, full screen).
- New: **the "Management" tab** (Windows servers, group "Network and protection"): presets, colours (24 bit, 16 bit, 256, grey), the frame rate (1-30), the traffic limit in Mbit/s, "lower the quality by itself", the pointer, Ctrl+Alt+Del; the remote desktop (RDP) **on and off** (a confirmed action; while it is off, or for Windows Home, the "Desktop" tab and menu items are hidden, and they come back at once when it is turned on); the Windows sessions now.
- New: **the view "Console" ("Пульт")** next to "Cards" and "Map" (not on a phone): no summary tiles or filters, a side list of servers by groups with a switch of what a click opens (terminal, RDP, Assistant), tabs of the open sessions beside it, and a right click "Files" that opens the whole "Files" tab in a floating window. **The sessions belong to the Console**: they stay connected while you switch between the tabs, look at the cards or open a server page; a hidden Assistant is paused. The windows are listed at the bottom of the side list; the open tabs, the mode, the folded groups and the Files windows are remembered and come back after a reload.
- Not done: a terminal that outlives the tab (it needs a persistent shell on the agent), the folder of a terminal or of a Files window after a reload, a pause for a hidden RDP tab.
- The details of every step are in the entries of the betas below.

## 0.5.0-beta.2

- Fix: the monitor icon of the display button of the Assistant was small in the tab (and in the Console). The small icon (13.5 px) is only for the header of a floating window, whose text is 11 px; in a tab the icon is 18 px again (`ActionMenu` got `iconSize`).

## 0.5.0-beta.1

- New (beta): **the view "Console" ("Пульт")** next to "Cards" and "Map" (not on a phone): no summary tiles or filters, only the side list and the window. At the top of the side list a switch of what a click opens (terminal, RDP, Assistant: an icon and the number of servers that have it; RDP only for Windows with RDP on, the Assistant for Windows). Below, the servers by groups (a group folds) with a status dot, the name and the mark LIN or WIN; a server that is offline is grey and does not open. A click opens the session in a tab beside the list (not a floating window), above it the tabs of the open sessions (with the buttons "<" and ">" when they do not fit); a second click on the same server in the same mode shows the tab that is open. **The sessions belong to the Console, not to a server page:** they stay connected while you switch between the tabs, look at the cards or open a page of a server, and a terminal keeps what is on its screen. The hidden Assistant is paused (a new frame, `Pause`: the computer stops taking pictures until the tab is shown), RDP is not.
- New: **Files in a floating window.** A right click on a server in the Console: "Files" opens the whole "Files" tab (tree, listing, create, upload, download, copy, move, archive, the editor in its own window) in a floating window, one per server; "Open the server page" is next to it. The windows (these ones, the terminals, the editors) are listed in a separate area at the bottom of the side list ("Windows · N"): a click minimizes or restores, a cross closes; the dock of floating windows is hidden while the Console is on the screen.
- New: **what you worked on is remembered.** The open tabs and the tab in front, the mode of the side list, the folded groups (`localStorage`, `linager.pult.state`) and the Files windows with their place and size (`linager.pult.files`) come back after a reload or a sign-in; the sessions reconnect themselves. A terminal opens again in the home folder (its screen is not restored: the shell ends with the connection). Not done: a terminal that outlives the tab (it needs a persistent shell on the agent).
- Fix: the monitor icon of the display button in the header of the Assistant was too big next to the 11 px text of the buttons. A pixel icon in a small button is drawn at 13.5 px (`PixIcon` got a `size`; 1.5 px per square, exactly 3 device pixels on a 2x screen), the other icons stay at 18 px.
- Dev: the simulator (`linager-sim -windows`) introduces itself with a Windows OS name (`helloOS`), so the Windows hosts are Windows in the panel without a manual fix.

## 0.4.0-beta.5

- Fix: **the buttons in the header of the Assistant window were bigger than those of the Desktop and the terminal** (13 px text instead of 11 px). The menus of the session, the display mode and "More" are small buttons now, as the neighbours are; `ActionMenu` got a `compact` option, the "More" menu of the Desktop window uses it too. The icon of the display button does not make its button taller.
- Fix: **in the mode "Window fits the picture" the bottom of the picture (the taskbar of Windows) went under the stats line** when the picture was taller than the page: the window was cut to the page and the picture, shown at 100%, was cut with it. Now a picture that does not fit is scaled down as a whole and the window takes its proportions, so nothing is cut off. The window also measures what it needs around the picture again if the real picture area comes out another size (the stats line wraps in a narrow window).

## 0.4.0-beta.4

- Change: **the toolbar of the Assistant.** The session button just says "Session" (the list still describes every session). A new button with a monitor icon holds the **display mode**: in a floating window "Picture fits the window", "Window fits the picture" (the default: the window gets the size of the picture at 100% and follows it when the picture changes, as far as the page holds it) and "Window fills the browser"; in the tab "Picture fits the window" and "Picture at 100% (scrolls)"; "Full screen" is in both. Resizing the window by hand leaves the "Window fits the picture" mode, the maximize button of the header is the same as "Window fills the browser". The items "Scale" and "Full screen" are gone from the "More" menu. `ActionMenu` can show an icon.
- Change: the "Configure" link under the screen of the Assistant is gone in the tab too (it was removed from the window in beta 1); the way to the settings is the item "Quality and frame rate..." of the "More" menu and the "Management" tab.

## 0.4.0-beta.3

- Change: **the pointer of the computer is not drawn into the picture by default** (the setting is now `show_cursor`, off unless the box in "Management" is ticked; an earlier record keeps the pointer out too). Your own pointer is shown over the screen and moves without delay.
- New: **the clipboard works with the usual shortcuts** in the Assistant: Ctrl+V sends the text of your clipboard (taken from the paste event of the browser, which needs no permission, also over HTTPS) and then presses V on the computer; Ctrl+C and Ctrl+X fetch what was copied there into your clipboard. On a Mac Cmd works as Ctrl for all keys (a Cmd tapped alone is the Windows key). The menu items stay.
- New: **the "Desktop" tab and menu items are hidden while the remote desktop (RDP) is off** (and for Windows Home, which has none): the server page and the menu of a tile and a row on the dashboard. The Windows agent reports `desktop_off` with every sample (the registry only: `fDenyTSConnections`, the edition), so the change shows on the dashboard in a few seconds; turning RDP on or off in "Management" shows or hides the tab at once, without reloading the page.
- Default of the picture: a new computer, and any whose settings were never changed, is on the "Normal" preset (24 bit, 15 frames, 4 Mbit/s, lowering the quality by itself, no pointer in the picture).

## 0.4.0-beta.2

- New (beta): **the Assistant, stage 2.** The helper follows the desktop Windows sends the input to (the thread is attached to it with `OpenInputDesktop` and `SetThreadDesktop`, every Windows call goes through one locked thread), so **the lock screen, the sign-in screen and UAC prompts are seen and controlled**; a badge "Lock / sign-in / UAC screen" shows in the stats line and the header (`Stats.desktop`). **Ctrl+Alt+Del**: the agent takes it out of the stream and sends the secure attention sequence itself (`SendSAS`, a service only), on the console session; the button is off for other sessions. The "Management" tab got a switch for the Windows policy "Software SAS generation" (`assistant.sas`, confirmed, audited) because Windows ignores the sequence from a service without it. **Text clipboard** both ways on request (up to 1 MB, menu items "Send my clipboard" / "Get the computer's clipboard"; the browser writes the answer only if it asked for it; the audit entry of the close counts them). **Several monitors:** the "More" menu picks a monitor or all of them as one picture (the Hello frame lists them, `Monitor` frame 20 chooses; mouse positions are mapped over the virtual desktop, DPI awareness is per monitor). 
- New: **an option to leave the pointer of the computer out of the picture** (`hide_cursor` in the settings, a box in "Management"): the browser then shows its own pointer without delay; with the option off (default) the canvas hides the local pointer, which stage 1 did not.
- Change: the helper loop works on one `assistHost` (picture, input, displays, clipboard, desktop, pointer), implemented by Windows and by the drawn computer of the simulator and the tests (two displays, a clipboard, a lock screen that Ctrl+Alt+Del switches). The relay is no longer blind: it reads the frames that go up to take out Ctrl+Alt+Del. Not verified on a real Windows computer: the switch to the Winlogon desktop, `SendSAS` and the policy, the clipboard of another session, a second display.

## 0.4.0-beta.1

- New (beta channel): **the Assistant for Windows computers, the view and control of the console or of an RDP session in the browser**, like Aspia or Radmin. It is not a new RDP session: the person at the computer stays, Windows Home works, no password is asked (access follows the Linager rights, the new operator module "Assistant", off by default; no consent prompt, every opening and closing is in the audit log). A tab "Assistant" next to "Desktop" (with the "open in a window" button), a button on the cards of Windows servers on the dashboard and an item in the menu of a tile and a row. The session is chosen from a list (the console and the active RDP sessions; a disconnected session has no screen). How it works: the agent starts a helper (`linager agent assistant-helper`, the same file) in the session with a copy of the service token moved to that session; the helper takes the screen with GDI, compares it in 64x64 tiles and sends the changed tiles as JPEG over the stream of the agent; the mouse and the keys come back (scan codes, `SendInput`). The helper calls the agent back on the loopback with a one-time token. A new package `internal/assist` holds the wire format and the encoder (colour reduction, tile comparison, traffic bucket with adaptive JPEG quality), the loop of the helper is independent of Windows and runs over a drawn screen in the simulator and in the tests.
- New: **the "Management" tab** (group "Network and protection", Windows servers): presets, colours (24 bit, 16 bit, 256, grey), the frame rate limit (1-30), the traffic limit in Mbit/s or none, "lower the quality by itself"; saved per computer (migration `00018`, `servers.assistant`) and sent to the sessions that are open. The same tab turns the **remote desktop (RDP) on and off** (a confirmed action; off forbids new connections and closes the firewall rule, open sessions stay; the agent and the Assistant do not depend on RDP) and lists the Windows sessions.
- Limits of this version: no lock screen and UAC prompts, no Ctrl+Alt+Del key in practice, no clipboard, only the primary monitor, no sound, the pointer is drawn into the picture. The helper runs as SYSTEM: an antivirus may need an exception for `linager.exe`. Not verified on a real Windows computer (the CI checks the structures, the encoder and the relay; the start of the helper in a session, the screen and the input are for the test machine).
- Dev: `agent.RunOptions.ExtraStreams` adds stream handlers (the e2e test of the panel runs the Assistant over a drawn screen); the Windows simulator (`linager-sim -windows`) offers the Assistant with sessions, a drawn desktop and a working RDP switch.

## 0.3.13

- Fix: **the network charts (Overview, Metrics) were empty on LXC containers** although the interface table showed the speed and the traffic. The total for the charts is the sum over the physical interfaces (so that a Docker host does not count a bridge and veth ends twice), and an interface counted as physical only if sysfs had a `device` entry for it; the `eth0` of a container is one end of a veth pair and has none, so the sum stayed 0. Now a host with no physical interface counts the interface of the default route (`/proc/net/route`, the lowest metric), and without a default route every interface that is up and not virtual by name. Hosts with a physical card are not changed. Tests: the choice for a host with a card, a container with and without a default route, a stale default route; the parsing of the route table.

## 0.3.12

- Fix: **the summary tiles of the dashboard shrink together with the scroll, and the servers do not go under the filter bar until they have.** The first 54 px of a wheel scroll over the list go into the shrinking (the hints fold away, the number gets smaller, the padding goes down; a number property `--p` from 0 to 1 drives it and is smoothed over 0.12 s, so a step of a mouse wheel is not a jump); the rest scrolls the list. Scrolling back up to the top of the list grows the tiles again. The list's bottom edge follows frame by frame, so the page does not jump. Keys, the scroll bar and touch (no wheel) snap the tiles to the strip. Nothing changes when the list is too short to scroll, on a phone, or with "reduce motion" (no smoothing).

## 0.3.11

- Change: **the four summary tiles of the dashboard shrink to a thin strip while the list of servers is scrolled** (the label and the number in one line, no hints); at the top of the list they are full again. On a phone they stay the strip of four cells they already were. The list takes the room the tiles free; the change is made with two thresholds, so the strip does not flicker at the edge, and only when there is enough list left to scroll.
- Fix: on a phone the close button of a dialog is the same pixel icon (18 px) as the other header buttons, not a small text "×".

## 0.3.10

- New: **a badge with the number of Docker updates in the side menu of the server page**, next to the "Docker" item (and in the "Sections" sheet and the bottom bar on a phone), like the one of the "Updates" item. It counts the containers with a newer image (`Sample.ImageUpdates`, ignored containers are not counted), hidden at 0 or while unknown; the tooltip says "Docker images with updates: N".
- Change: **shorter badges on the tiles of the Map view.** "reboot required" is `R!` (`П!`), "updates: N" (packages or Windows updates) is `U↑: N` (`О↑: N`), "images: N" (Docker) is `D↑: N` (`Д↑: N`), both agent badges (update available, outdated) are `A!` (`А!`; the colour tells them apart); the full text is the tooltip. The other badges and the card view are unchanged.

## 0.3.9

- Docs: **the README (en, ru) and the landing page are brought up to date.** New: the Windows agent with the remote desktop, editing a Docker container, ignoring updates, the top bar with the menu, the phone layout, the Linux and Windows install links. A block about Windows and a block about the phone layout on the landing, a Windows and an "Add server" tab in its screenshot gallery, "Windows and adding servers" and "Docker: editing a container" in the README. The "new in 0.1.69" labels are gone, and Windows is no longer called beta (README, landing, site, docs).
- Docs: **all screenshots (`docs/screenshots/en` and `ru`) are taken again** on the 10% smaller interface with the new top bar and icons: dashboard (light and dark), server page, metrics, terminal, files, updates, Docker, firewall, security, users, network, the access restriction and its journal, the phone. New: `windows-overview`, `windows-disk`, `windows-users`, `windows-updates`, `docker-edit`, `add-server`.
- Dev: the agent simulator (`linager-sim -windows`, built with `-tags sim`) enrolls simulated Windows hosts (a Windows OS name, the Windows answers of users, updates, disk, network and services); the sample data of Docker is more like a real host, the history of the disk no longer sits at 100%. A normal build is not affected.

## 0.3.8

- Change: on the server page the note, reboot and delete buttons of the top bar stand next to the menu button, to the right of the "live" clock (they were to the left of it).

## 0.3.7

- New: **editing a Docker container.** The menu of a container has an "Edit..." item with a window of five tabs: Main (name, image, restart policy, command), Ports, Volumes, Environment, Resources (memory and CPU limits). Every field is marked "in place" or "recreate". The name, the restart policy and the limits are changed in place, the container keeps running; the image, the command, ports, volumes and variables recreate it the same way "Update image" does (stop, rename, create, start, check; if anything fails the old container comes back). Before applying, the window lists the changes and asks for confirmation; the work is shown in a live log. The checks (names, image, ports including ports taken by another container, absolute paths, variable names) are made in the window and again on the agent. A compose container changes only the restart policy and the limits. The settings are sent in a request body and the stream carries a one-time token, so passwords in variables are not in URLs or in the audit log (it gets the names); the action is journaled as `docker.container.edit`. Tests: the command split/join, the checks, the plan (in place or recreate, compose), reading the settings, the in-place update, the recreate with ports/volumes/variables, a port taken by another container, the rollback, and the one-time token. See `docs/*/modules.md`.
- Dev: the image update (`plainUpdate`) and the editor share one replacement routine (`swapContainer`); the simulator (`linager-sim`) answers `docker.config` and `docker.edit`.

## 0.3.6

- Fix: **0.3.4 and 0.3.5 were published without the Windows files** (the MSI and the Windows agent): a new test did not build on Windows (0.3.4), then one of its checks (the file mode 600) does not apply to Windows (0.3.5), so the Windows part of the release failed, and the rest was published anyway. This release has the changes of 0.3.4 and all the files. Use 0.3.6 instead of 0.3.4 and 0.3.5.

## 0.3.5

- Fix: the tests of the Linux-only counting of updates live in a file for Linux only. (This release still lacks the Windows files, see 0.3.6.)

## 0.3.4

- Change: **the server page has its header in the top bar.** The back button, the name and the state of the server stand next to the logo, and the note, reboot and delete buttons are icons on the right (with tooltips). The block under the top bar and the line "Debian... · address · linager-agent..." are gone: the system is in the "System" card of the Overview, the address in the table of the interfaces, the version at the bottom of the side menu. The "last seen" time of an offline server is the tooltip of its badge. The phone keeps its own header.
- New: **ignoring updates of single packages and Docker containers.** The "Actions" menu of a package (Updates tab) and the menu of a container have an "Ignore" item, like the services: the item stays in the list (dimmed, marked "ignored") but is not counted in the dashboard, in the badges of the side menu and in the "can be updated" numbers. The agent keeps the lists (`ignored.json`, mode 600) and counts without them, so it works with the real numbers; "Install all" still installs everything, "Update marked" skips ignored containers. The actions are in the audit log. Tests: the lists survive a restart and are private, bad names are refused, ignored packages and containers are left out of the counts, the command.

## 0.3.3

- Change: **the whole interface is about 10% smaller**, on the computer and on the phone. Done in whole pixels, not with a zoom, so the 2 px lines, the shadows and the pixel font stay sharp: the font sizes go down by a step (12 to 11, 13 to 12, 14 to 13, 16 to 14), the paddings, gaps, widths and heights by a tenth. On the phone the buttons and rows keep their 44 px touch size and the fields keep 16 px (iOS would zoom the page otherwise).
- Change: **the icons are redrawn on a 9x9 grid** and shown at 18 px, so every square is exactly 2 px and the icons are sharp on 1x, 2x and 3x screens (the old 8x8 icons at 20 px had 2.5 px squares and were blurred on some screens). The icons of the card buttons and of the terminal are shown at 16 px for the same reason.
- Dev: the agent simulator (`linager-sim`, built with `-tags sim`) answers the commands of the modules (services, processes, updates, network, firewall, security, Docker, disk, users, logs) with sample data, so every tab can be looked at without a Linux host. A normal build is not affected.
## 0.3.2

- Fix: **an agent update from the panel failed with "context deadline exceeded" after exactly 2 minutes** on a slow link. The agent gave every command two minutes, the download of the release (about 35 MB) included. Now the update command has **10 minutes** (`protocol.AgentUpdateTimeout`), the other commands keep two; the server waits for the answer 12 minutes instead of 4. Tests: the limit of the update command and of another one.
- New: **a detailed journal of an update**, to find out why it is slow or fails. The agent (`journalctl -u linager-agent`) writes `update: …` lines: the start (versions, architecture, time left), the answer to the binary request and how long it took, the progress of the download every 10 seconds (bytes, percent, average rate, rate of the interval, seconds without a byte), every stage with its length, and on an error the stage, the reason and whether time ran out; also `command timed out` for any command that hit its limit. The server (`journalctl -u linager`) writes `agent update: …` lines (the command and its wait, the stages the agent reported, a drop of the connection during the update with the step, the agent coming back on the new or the old version, the confirmation, an error, a rollback) and, for every release file given to an agent, `release file: sent` with the bytes, milliseconds and rate, or `the transfer did not finish`. See `docs/*/updates.md`. Tests: the stages, a failed stage, a timeout, a refusal and the download progress in the journal.

## 0.3.1

- Change: **the top bar of the dashboard.** The view switch (Cards, Map) moved from the page into the top bar: view switch, Add server, menu. The menu button (three lines) opens a list: theme and language (they switch in place and keep the menu open), Edit, Settings (Profile) and a red Log out. While the dashboard is being edited, a **Done** button stands in the top bar. The line "27 servers" above the summary is gone (the count is in the Online card). On other pages and on the sign-in screen the top bar has only the menu; on a phone the bottom bar and its "More" sheet stay as they were, the view toggle and Add server are in the top bar.
- Fix: **"invalid mount" in the Disk tab of a Windows server** when counting the used space: the panel accepted only mount points that start with "/", and a Windows volume is "C:". Test: `TestValidMount`.
- Fix: the Users tab of a Windows server had a wide horizontal scroll: a long description of a built-in account (it was never wrapped) widened the table. Descriptions and group names wrap now.

## 0.3.0

The first stable release with the **Windows agent** (beta feature, see `docs/en/windows.md`). It gathers everything of 0.3.0-beta.1 to beta.15 below; there is no database migration, a server and its Linux agents update as usual.

- New (beta): the agent for Windows x64 (10/11, Server 2016 and newer) as a Windows service with an MSI package: metrics, services, processes, event log, terminal (ConPTY), files, disk, users, Windows Update, network settings with a rollback, reboot, and a **remote desktop in the browser** (RDP through the panel, no extra software). The agent updates itself from the panel; the dashboard cards and the map tiles show a Linux or Windows badge.
- New: install of a Windows agent by a link, one PowerShell command (the panel gives a command for the outside address and one for the local network); the server has a second ECDSA certificate for Windows clients, which cannot verify Ed25519.
- Fix: releases with a label such as `beta.11` are compared by number (they were taken for older than `beta.9`).
- Fix: the external install link takes the site address, not the bare host name of the server; the local one prefers an IP address.
- Change: a new host whose clock differs from the server's by more than two minutes is refused with a clear message instead of "bad signature". **Set the time and the time zone of a new computer before the install.**

## 0.3.0-beta.15

- Fix: **the "external" install link used the host name of the server** (`https://homeSRV-linager:43752/...`) instead of the address of the site. A name without a dot was taken for a name from the internet, and it stood first in `agents.hosts`. Now a bare host name counts as local; the external command uses the site address (a domain or a public IP of `agents.hosts`, or the address the panel was opened with), and the local command prefers an IP address of the local network to a name. Test: `TestJoinHostsBareNameIsLocal`.

## 0.3.0-beta.14

- Fix: **the install link did not work from Windows PowerShell at all** ("Could not create the SSL/TLS secure channel"). Two causes: the certificate of the agent port has an Ed25519 key, which Windows (Schannel) cannot verify in TLS, and Windows 10 and Server 2019 have no TLS 1.3. The server now has a **second certificate with an ECDSA P-256 key** (`agents-ec.crt` and `agents-ec.key` in the data directory; a server that was installed before makes it at the start, for the same hosts). The listener offers both: Go picks the first one the client can verify, so agents and wget keep the Ed25519 certificate and its pins, and a client without Ed25519 gets the ECDSA one. The Windows install command pins the fingerprint of the ECDSA certificate, the script gives the agent the fingerprint of the main one. Tests: which certificate a client with and without Ed25519 gets, the fingerprints in the command.
- Fix: the Windows install command left its filter of certificates switched on for the whole PowerShell window, so every later HTTPS request in it failed ("could not establish trust"). The command now switches it off at the end, and the script does so before the installation.
- Docs: `docs/*/windows.md` tell how to install from a local file (a computer that cannot reach the panel over HTTPS, or an offline one).

## 0.3.0-beta.13

- Fix: the Windows install command failed with "Could not create the SSL/TLS secure channel". The agent port accepts only TLS 1.3, and Windows PowerShell on older systems (Windows 10 before 2004, Windows Server 2019) has none, and the command only asked for TLS 1.2. Now the command asks for TLS 1.3 and 1.2 (the first where the system has it), and **the agent port serves a client that cannot speak TLS 1.3 with TLS 1.2, without a client certificate, and only on the install link routes** (`/v1/join/...`); every other route answers such a client with 403, and agents always use TLS 1.3, so the protection of the agent connection is unchanged. Tested: a TLS 1.2 client gets the install script and is refused on the agent routes.

## 0.3.0-beta.12

- Fix: **a release such as `beta.11` was taken for older than `beta.9`.** The label after the dash was compared as text ("beta.11" is below "beta.9" letter by letter), so a server on beta.9 did not offer beta.10 or beta.11, and an agent refused an update to them as a "downgrade". The numbers in the label are now compared by value, like semantic versioning does (beta.9 < beta.10 < beta.11, a number below a word, a label without a number below one with a number). Test: `TestPreReleaseNumbersAreCompared`. **A server or an agent of beta.9 or older still has the old comparison**, so it cannot update to this release from the panel: update the server by hand with `sudo linager update --version v0.3.0-beta.12 --allow-downgrade`, and an agent by installing the new MSI over the old one (or, on Linux, `linager agent update --version v0.3.0-beta.12` after the server is updated; the agent of a beta older than beta.10 may need the same by hand). From this release on the updates from the panel work as usual.

## 0.3.0-beta.11

- New (beta): **the "Disk" tab for Windows servers**, the same tab as on Linux: the volumes with a forecast of when they fill up (the agent keeps a history), the tree of folders and the pie (system and programs, user profiles, temporary files, other, free), and the confirmed ways to free space: temporary files, the downloads of Windows Update, the Recycle Bin, the Delivery Optimization cache, crash dumps and reports, `Windows.old`. The Windows agent speaks the Linux protocol (`disk.state`, `disk.scan`, `disk.clean`); the server allows the new kinds of cleanup.
- New (beta): **install of the Windows agent by a link: one PowerShell command.** The tab "Install by a link" of "Add server" has a Linux or Windows switch. The Windows command checks the certificate of the server by its fingerprint (a small C# class in the command), downloads the installer package from the server (`/v1/join/<link>/msi`), compares it with the signed release, writes the one-time code of the link to a file that only SYSTEM and the administrators can read, installs with `msiexec` and starts the service; the service pairs with the code, so the computer appears in "Confirmations" ready to be confirmed in one click, with no key and no `enroll.txt`. The script is served at `/v1/join/<link>/windows` (readable first). The release manifest has a new entry `windows_msi` for `linager_windows_amd64.msi`, cached by the server like the other files.
- Change: the **Users** and **Updates** tabs of Windows servers look like the Linux ones: the users as a table with a search, the filters (enabled, administrators, signed in now), the groups as tags and a menu of actions in the row; the updates with the number and the security count, the buttons "Check", "Install security" and "Install all", the table with a filter and the log of the installation. The sessions are a table below the users.
- Tests: the Windows disk (volumes, tree, categories, refusals), the pin of the install command against a TLS server in Windows PowerShell on a Windows runner (the right fingerprint is accepted, another is refused), the text of the command and of the script, the Windows file and the MSI in the cache. **Not checked:** a real install by the link from a new computer through your panel (with a release that has the MSI), the cleanup of `Windows.old` and of the update downloads on a real machine.

## 0.3.0-beta.10

- Fix: the Windows tabs had red buttons with red text, so the confirmation button and the "Delete" and "End session" buttons showed no label. The confirmation is the solid red button with a readable label again; the row buttons of dangerous actions are outlined. Checked in the browser.
- Change: the adapters of the Windows "Network" tab look like the Linux ones: a card per adapter with its name, the tags UP or DOWN, physical or virtual, DHCP or static and the address in the folded line; the open card has the facts (description, MAC, MTU, mode, IPv4, gateway, DNS, speed) and the edit form inside it instead of a window.

## 0.3.0-beta.9

- New (beta): **Users, Updates and Network tabs for Windows servers.** *Users:* local accounts and groups (add with a required password, set a password, enable or disable, change groups, delete) and the signed-in sessions with "End session"; the built-in administrator (SID -500) and the last enabled administrator are protected. *Updates:* Windows Update on the computer itself (title, KB, size, severity, security), install the selected or all updates with a live log, the number of updates and "restart required" on the dashboard (searched once an hour). *Network:* the adapters (state, DHCP or static, IPv4, gateway, DNS, MAC, speed, MTU) and the change of one adapter with **an automatic rollback after 30 seconds unless confirmed**, done by the agent through a one-shot Task Scheduler task and by `recoverNetChange` after a restart. New agent modules `users`, `winupdate`, `network` (Windows only; an agent of beta.8 or older gets them with the update from the panel); one server route `POST /api/servers/{id}/win/{module}/{action}` with the table of allowed actions, the operator rights `users`, `apt`, `network`, and audit entries `win.users.*`, `win.network.*`. Docs: `docs/en/windows.md`, `docs/ru/windows.md`, section 12o of the architecture.
- Tests: the validation of names, groups and network changes (every platform); on a Windows runner the whole life of a local user (create, groups, disable, password, delete, refusal to delete the built-in administrator), the adapters, the confirmation rules, the search of Windows Update. **Not checked:** installing updates and changing an adapter (a real rollback after a cut connection) on a real machine; take a snapshot of the virtual machine before you try the network change.

## 0.3.0-beta.8

- New: the button "open in a window" next to the item "Desktop" in the side menu of a Windows server, like the one next to "Terminal". It opens the remote desktop in a floating window (one per server). Checked in the browser.

## 0.3.0-beta.7

- Fix: the remote desktop showed a black screen after the sign-in: the screen of the RDP client stays hidden until the page asks it to show (`setVisibility(true)`), and the page did not. Found on the first real session. Only the screen was affected: the connection, the sign-in and the audit rows were fine.

## 0.3.0-beta.6

- New (beta): **remote desktop of a Windows server in the browser.** The tab "Desktop" (after "Terminal"), a floating window like the terminal and the item "Remote desktop" in the context menu of a Windows tile on the map. The RDP client (IronRDP, WebAssembly, MIT or Apache-2.0; `@devolutions/iron-remote-desktop` 0.11.0 and `-rdp` 0.7.0, pinned) is part of the panel and loads when the desktop is first opened (about 2 MB compressed); nothing is installed. The browser talks RDP to the panel over a WebSocket (the protocol RDCleanPath, in `server/rdcleanpath.go`), the panel carries it through the agent to the Remote Desktop service of the computer (`127.0.0.1:3389`, the port from Windows): the port is not opened to the network. The person signs in with a Windows account; the client signs in itself (CredSSP), the password is not stored or logged, only the login is remembered in the browser. The bar of the window has **Ctrl+Alt+Del** and a "More" menu: Task Manager, Windows key, Win+L, Win+R, Alt+Tab, text clipboard both ways, scale (fit or 100%), the resolution that follows the window, full screen, disconnect. New agent module `desktop` (Windows only) with the state of the remote desktop and the confirmed action "Turn remote desktop on" (the setting, the firewall group and the service); new right `desktop` for operators (off by default); the audit log has `desktop.open`, `desktop.close` (with the login as typed and the duration) and `desktop.enable`. One desktop per computer at a time. The content security policy now allows WebAssembly (`wasm-unsafe-eval`) and `data:` in `connect-src` (the client loads its module from a data address). The scripts and styles of the panel are sent compressed (cached gzip), so the 6 MB client is 2 MB on the wire. Docs: `docs/en/windows.md`, `docs/ru/windows.md`, `docs/design/ui-spec.md`, section 12o of the architecture.
- Tests: the RDCleanPath handshake and relay against a fake RDP server over TLS (a refusal without TLS, a wrong version, garbage, no request), the DER layout, the TCP pump of the agent, the compressed assets. Checked in the browser against the real server: the window and the form, the WebAssembly client loading under the new policy, the connection reaching the proxy and the agent, the audit rows. **Not checked: a real Windows session** (login, picture, input, clipboard, Ctrl+Alt+Del): the Mac has no RDP server and the Windows runner has no desktop to show. This first run on a real machine is yours.

- Fix: the badge "Linux" / "Windows" is now also on the tiles of the **Map** view of the dashboard (beta.5 had it on the cards only). It sits in the top right corner of the tile next to the name, which is cut with an ellipsis when it does not fit. Checked in the browser.

## 0.3.0-beta.5

- New: **a badge "Linux" or "Windows" in the corner of every server card** of the dashboard (the card view and the phone card; in the edit mode it sits after the name, next to the edit buttons). The platform comes from the OS name the agent reported (a Windows name starts with "Windows"); the Windows badge uses the accent colour, the Linux one is muted; the full OS name is in the tooltip. The map tiles and the list are not changed. Checked in the browser against the real server with one server marked as Windows. Docs: `docs/design/ui-spec.md`.
- This release is also the target for the first update of a Windows agent from the panel (`0.3.0-beta.4` to `0.3.0-beta.5`): the agent is replaced by the signed exe, the stages are shown on the Updates page, and an agent that does not reconnect rolls back by itself.

## 0.3.0-beta.4

- New (beta): **the panel updates a Windows agent.** The release manifest has a new entry `windows_amd64` (not an architecture, so servers and agents of earlier releases ignore it) for `linager_windows_amd64.exe`; the server caches the file with the others, verifies it again before every distribution and gives it to the agent at `binary?arch=windows_amd64`. The agent checks the signed manifest and the checksum, saves the running exe as `linager.prev.exe`, moves the running file aside (Windows can rename a running program), puts the new one in its place and restarts the service with a helper process. The stages and the Updates page are the same as for Linux agents. **Rollback:** a one-shot task of the Task Scheduler (`LinagerAgentRollback`, SYSTEM, after 70 s) runs the old exe, which stops the service, restores the old file and starts the service; the new version cancels the task with its first connection to the server. By hand: `linager.exe agent update`. Agents of beta.1 to beta.3 cannot update this way (they have no code for it): the table says "new MSI X" until the MSI of beta.4 or newer is installed once. The product version in the Windows program list stays that of the MSI after a panel update (cosmetic). Docs: `docs/en/windows.md`, `docs/ru/windows.md`, section 12o of the architecture.
- Fix: Windows network counters: some drivers (for example the emulated Intel PRO/1000 of a hypervisor) leave the total in `InOctets` / `OutOctets` empty; the agent now takes the larger of the total and the sum of the unicast, multicast and broadcast octets, from the normal and the raw adapter table, then the old 32-bit counters. If an adapter that is up still reports no bytes, the agent writes it once to `agent.log`. Checked on a Windows runner; on the reporting machine Task Manager shows zero too, so it may be the virtualization.
- Tests: the Windows file in the cache (download, verify, tampered file, a release without it), the endpoint choice, the by-hand rule for old agents, the stage and checksum tests of the updater now run on Windows too, and on a Windows runner: replacing a file that is open, restoring the old file, the real scheduled task (create, find, delete), the rollback command, and the service restart command in the MSI smoke test. **Not checked:** the whole update from the panel on a real machine (the restart started from inside the service, a failed update that rolls back).

## 0.3.0-beta.3

- Fix: Windows network counters (rates and totals of the adapters) were still missing on some machines. The agent now reads the whole table of adapters in one call (`GetIfTable2Ex`) and matches it to the adapters by index and by name, falling back to a call per adapter. If an adapter that is up still has no counters, the agent writes the adapter and the reason once to `C:\ProgramData\Linager\agent.log` (search for `network counters`): send those lines if the numbers are still empty. Checked on a Windows runner (two adapters, counters present). The reason on the machine that showed no numbers was not found.

## 0.3.0-beta.2

- Fix: the Windows agent now reports network metrics. The counters of each adapter are read on their own (`GetIfEntry2Ex`, with the link speed), so one adapter that does not answer no longer takes all the others with it. Checked on a Windows runner.
- Fix: a page opened before an update asked for script files of the old build, and the server answered with the page of the application (status 200) instead of an error, so a window that loads its code on demand (the file editor) stayed empty. A missing file in `/assets/` is now a 404, and `index.html` is sent with `Cache-Control: no-cache`, so a new release is picked up at once. Test: `TestStaticMissingAssetIsNotThePage`. This is the likely cause of the empty editor window reported on the beta; it was not reproduced, so if it still happens after a hard reload of the page, the browser console tells more.

## 0.3.0-beta.1

- New (beta): **the agent for Windows** (x64: Windows 10 / 11, Server 2016 and newer). It connects to a Linux server like the Linux agent and has the modules that map onto Windows directly: metrics and inventory (CPU, memory, disks, adapters, uptime, processes, "restart required"), processes (list, end), services (list, start, stop, restart, startup type, through the Service Control Manager), logs (Windows event logs, followed live), the terminal (PowerShell in a ConPTY pseudo console, as SYSTEM), files (the drives at the top of the tree, `/C:/Users/x` paths) and reboot. Apt, disk cleanup, network, firewall, security, users and Docker are Linux-only: their tabs are hidden for a Windows agent. Install with the MSI package (`msiexec /i linager_windows_amd64.msi /qn SERVER=host:port`): it registers the service `LinagerAgent` (SYSTEM, automatic start, restarted after a failure); the service pairs with the server by itself and writes the key and the check code to `C:\ProgramData\Linager\agent\enroll.txt`. Keys and state are kept in a folder open to SYSTEM and the administrators only (DACL, the counterpart of mode 600). The panel does not update a Windows agent: the table on the Updates page says "new MSI X", the update is a new MSI installed over the old one. Docs: `docs/en/windows.md`, `docs/ru/windows.md`, section 12o of the architecture.
- Change: the code is split by platform with build tags. The Linux-only files of the agent got `//go:build !windows`, the registration of modules moved to `registry_unix.go` and `registry_windows.go`, the Windows build of `cmd/linager` is the agent only. Linux behaviour is unchanged (module lists, protocol and migrations are the same; the next migration is still `00018`). A new dependency: `github.com/shirou/gopsutil/v4`, used on Windows only.
- Release: `windows.yml` (called by `release.yml`, can be run by hand) tests the Windows code on a Windows runner, builds `linager_windows_amd64.exe` and the MSI (WiX v4), installs the MSI, checks that the service runs and removes it. Both files are added to `SHA256SUMS` and signed; the manifest does not describe them. A failed Windows job does not block the Linux release. CI also vets and builds the Windows agent on every push.
- Tests: the path, event-log query, OS name and dangerous-service logic (every platform, `winutil_test.go`); metrics, services, processes, files, the terminal, the module list (Windows runner, `windows_test.go`); Linux tests are unchanged. **Not checked on a real Windows machine or a fleet**: pairing through the MSI, long-running behaviour, other Windows versions. Not in this beta: updating the agent from the panel, one-click install by a link, arm64, code signing (SmartScreen or an antivirus may warn).

## 0.2.9

- Fix: in the terminal window the buttons "User" and "Snippets" were taller than the other buttons; they are now the size of the buttons in the window of the file editor.
- Fix: the side menu of a server stays in place under the top bar while the page scrolls, and it is never taller than the free part of the screen, so it no longer makes the page scroll by itself when the panel on the right is short. A long list of tabs scrolls inside the menu, the footer with the agent version stays. On a narrow screen the menu is still the dropdown it was.
- Change: the one-off cleanup that 0.2.8 added for the rollback is removed: the migration `00017`, the cleanup of `agent.yaml` and `gateway.json` when an agent starts, the line in the dashboard that forgot the stored link style, their tests and the section 12o of the architecture. A database that is already at version 17 opens as before (checked: a database at version 17 opens with migrations that end at `00013`). An installation that went from 0.2.4 to 0.2.7 straight to this release keeps the old tables and the `gateway:` line; they are not used. The next new migration is `00018`.
- Docs: the mockups of the server page show an address from the private range instead of a public one.
## 0.2.8

- Change: **the Scheme view of the dashboard (0.2.4, 0.2.5) and the Gateway module (0.2.6, 0.2.7) are taken out again.** The project is back to the functionality of 0.2.3: the code, the tab, the module, the tests and the docs of those releases are gone (their entries below stay as history). Servers, metrics, users, notes and the ways to add a server are as they were in 0.2.3.
- Upgrade from 0.2.4 to 0.2.7 without leftovers. The migration `00017` removes what those releases added to the database: the tables `gateways`, `gateway_devices` and `gateway_events`, the column `dashboards.scheme` and the module "gateway" in the access of operators. Metrics, servers, users and the audit log are not touched (the entries `gateway.*` and `dashboard.scheme` stay in the log, it is history). The migrations `00014` to `00016` no longer exist; a fresh install goes from `00013` to `00017`, and the cleanup finds nothing to remove there. An agent removes its own leftovers when it starts: the line `gateway:` in the `modules:` block of `agent.yaml` and the file `gateway.json` in its data directory. The browser forgets the stored link style of the removed view.
- Tests: the cleanup migration (`TestRetiredLeftoversAreRemoved`) and the cleanup of an agent (`TestRemoveRetiredAgentModules`, `TestRemoveRetiredData`). Checked by hand: a database at schema version 16 made by the 0.2.7 binary, with a server, a metrics row, gateway data, a layout and an operator with the module, was migrated by the new binary to version 17: the leftovers are gone, the metrics, the server and the audit entry are intact, and the server starts on it.

## 0.2.7

- Fix: the gateway module of an agent could start a second pass right after the first one: a wake-up (a new configuration) that came before the loop of passes had read the configuration stayed in the channel and was taken for a new request. The loop now drops such a wake-up when it reads the configuration. The extra pass was harmless, only a repeated scan.
- Tests: the data races that `go test -race` found in the banner test of the gateway (pipe variables reused by goroutines) are gone, and the pass test no longer depends on the timing of the loop. The tests of the gateway were run 15 times with the race detector.

## 0.2.6

- New: **the Gateway tab** (stage 2a; group "Network and security", for agents with the new module `gateway`). The agent looks for devices in the networks the administrator chose, and the panel shows what was found: name, address, MAC and manufacturer, open ports with what the service said (the SSH banner, the title of a web page), how it was found, when it appeared and was last seen, and a journal of appeared and gone devices. Scanning is off until the administrator turns it on (a confirmed action, in the audit log); the agent's own networks (by its interfaces, VLANs labelled) are offered, other networks are added by hand (IPv4, from /30 to /24, up to 16; a larger network is offered to be split into /24; a public network only with an explicit confirmation, and it is scanned by ping and the short port list only, at a lower rate). Ways to find a device: ARP (own network), ping, a short list of TCP ports (a refused connection also shows a host is there), reverse DNS, banners. A device is "gone" after two passes in a row without an answer; the manufacturer comes from the IEEE OUI table built into the binary (about 40,000 entries; random MAC addresses have none); a device that is a server of the panel is marked "agent: name"; the administrator can give a device a name of its own and leave it off the map (the map comes in the next stage). Only administrators change anything; an operator with the module "Gateway" reads. The agent keeps its configuration and scans without a link to the panel; the panel and the agent check the networks by the same rules (`internal/gwnet`). Mockup: `docs/design/next/gateway.html`; docs: `docs/ru/modules.md` and `docs/en/modules.md` (module gateway), `docs/design/ui-spec.md` (tab 13), section 12p of the architecture.
- New (stage 2b of the Gateway): **names and the kind of a device.** For devices that are up the agent also asks over mDNS (the `.local` name and the announced services), NetBIOS (Windows and Samba names) and SSDP with the UPnP description (title, manufacturer, model, type; only in the network the agent is attached to, only from the address that answered, no redirects, 32 KiB at most), under the existing network option, now called "Look up names and types of devices"; none of it is done in a public network, and mDNS and NetBIOS also go through a router. The panel works out the kind of each device when it shows it (router, hypervisor, NAS, printer, camera, TV, speaker, Windows computer, phone, smart home device, server) with the reasons ("port 554, manufacturer Hikvision"), so better rules apply to data already collected; the table shows the kind as a tag and has a filter "All types", the device window shows the kind with its reasons, the mDNS services and the UPnP description. The agent marks the default gateway of the network as the router. Migration 00016. Docs: the module gateway in `docs/ru/modules.md` and `docs/en/modules.md`, tab 13 of `docs/design/ui-spec.md`, section 12p of the architecture.
- Tests (2b): the DNS wire format, NetBIOS answers, the UPnP description and its safety limits, mDNS and NetBIOS against fake responders on real loopback UDP sockets (agent); the table of kinds and the ingestion of services and UPnP through the API (server). Not verified on a real network: how real printers, TVs, Apple devices and routers answer, and SSDP multicast.
- Tests: the rules of the networks, the parsing of the ARP table and the routes, the banners, a pass over a fake network, the persistence of the configuration (agent); the configuration API with its checks, roles and audit, and the ingestion of passes (appeared, gone after two passes, a changed DHCP address, a replay, what the administrator set survives) on the server; a real-socket scan of loopback is available with `LINAGER_REAL_NET=1`. Not verified on a real Linux network: ARP, raw ICMP and the routing table (no reachable Linux host was available); the code was checked on loopback sockets, against a simulated agent and by cross-compiling for linux/amd64 and linux/arm64.

## 0.2.5

- Fix: the "Around" link style looked like "Tracks" for a fleet of more than 16 servers. The router worked only up to 16 servers and quietly gave the tracks above that; in the compact form it found no way at all (with 24 servers 0 of 24 links) because the rings of nodes were packed so close that the panel was walled in. Now "Around" spreads the nodes out so that there are corridors between them and works up to 60 servers (24 of 24, 58 of 60 links; about 0.2 s for 60 servers); above 60 the tracks are used and a note next to the switch says so. The layout with corridors is used only while "Around" is chosen. Tests: the router with corridors for 3 to 40 servers.

## 0.2.4

- New: **the "Scheme" view of the dashboard** (a third view next to "Cards" and "Map"; on a phone the single view button became three icons). The panel stands in the middle, the servers on rings around it, with the name and the state of each. Every link is drawn in the colour of the state and squares run along it from the server to the panel, slow, medium or fast by the traffic of the server (below 50 KB/s, below 2 MB/s, above). A click on a server lights its way to the panel and greys everything else; a strip under the scheme shows the details and the buttons "Open" and "Terminal". Servers, the panel and a whole group (by its label) can be dragged; the background shifts the view; zoom by the buttons "-", "+", "1:1" and "Fit", or with Ctrl/Cmd and the wheel. A large fleet is not turned into dots: the nodes keep their size and the scheme grows. The switch "Links" offers straight lines, right-angle links on shared tracks and right-angle links that go around servers and each other where possible (up to 16 servers). The places of the servers are saved on the server, for each user separately (migration 00014, `PUT`/`DELETE /api/dashboard/scheme`, returned by `GET /api/dashboard`); "Reset layout" returns the automatic one. Mockup: `docs/design/next/scheme.html`; docs: `docs/design/ui-spec.md` and section 12o of the architecture.
- Tests: the layout, the zoom, the tracks and the router (`web/src/lib/dashboard/schemeLayout.test.mjs`); the saving, the limits and the isolation between users on the server (`TestDashboardScheme`).

## 0.2.3

- Updates page, the table of agents: the "Stages" link under the progress bar of an agent that is being updated is gone (the row already says "step N of 8: name" and shows the bar; the full list of stages stays on the page of the server). The link remains for an agent whose update failed or was rolled back, where the list shows where it stopped.

## 0.2.2

- Docs: the README (en, ru), the landing page (en, ru) and the introduction of the site tell about the new ways to add a server (confirm a key, an install link, an install over SSH) and about the Linager menu that opens after an install. The landing got a "Three ways to add a server" block in the install section and a line in the install animation; the README got an "Adding servers" row in the table of features and a rewritten "Add a managed host" paragraph with the `wget … | sh` command.
- Fix: the note button next to "Reboot" (and the pencil next to the cross in the note window) could be a little taller or shorter than the text buttons, depending on the font metrics of the browser: the icon buttons had a fixed height while the height of the text buttons follows the font. The rows now stretch their buttons to one height (`align-items: stretch`), so the note button, "Reboot" and "Delete server", and the pencil and the cross, always have the same height and the same top edge. Checked in the browser with line heights from 14 px to 28 px.

## 0.2.1

- After an install the Linager menu opens, as if the person had typed `linager`: `install.sh` (the install from GitHub) and the script of an install by a link end with "Press Enter to open the Linager menu" (in Russian when `LANG` is Russian) and then run `linager` on the terminal. The prompt comes after the wizard's summary, so the addresses and the generated administrator password can be read first. The menu is not opened after a failed install, without a terminal (automation, `ssh` runs) or with `--no-tui`. `install.sh` no longer `exec`s the wizard; the link script removes its temporary files before it hands the terminal to the menu. Tests: the generated link script is valid shell, uses wget without sudo and ends with the menu; `install.sh` is valid shell and has the same ending; the ending was also run in a pseudo-terminal (prompt in both languages, Enter opens the menu, no terminal ends quietly, `--no-tui` skips it); `shellcheck install.sh` is clean.

- Install by a link: the command no longer has `sudo` (some systems do not have it; run it as root) and uses `wget` instead of `curl` (wget is in the base system of Debian and Ubuntu, curl is not): `wget -nv -O- --no-check-certificate --pinnedpubkey 'sha256//…' 'https://host:port/v1/join/lk_…' | sh`. `-nv` keeps the errors visible (an empty script piped into `sh` would end silently). The script of the link and the install over SSH download the binary with `wget` too (and install `wget` on the host when it is missing). Not verified on a real host: that `wget --pinnedpubkey` works together with `--no-check-certificate` (wget 1.21 on Debian 12/13 and Ubuntu 22.04+; wget refuses an unknown option, so an old wget fails instead of skipping the check).
- The note button on the server page and the pencil in the note window are the size of the other buttons of the project (36 px squares), and the empty note text has the missing space.

## 0.1.78

- New: **a note on a server.** On the server page, left of the "Reboot" button, an icon button opens the "Note" window with free text (up to 20,000 characters) in Markdown. The pencil next to the cross starts editing; "Cancel" and "Save" end the edit without closing the window (`Ctrl+Enter` saves, `Esc` in the editor cancels, the cross and a click outside refuse to close over an unsaved text). A small square on the button says that a note exists. Everyone who sees the server reads the note (on a phone the button is in the header), an administrator changes it. The Markdown renderer (`web/src/lib/markdown.ts`) escapes everything first and adds the markup afterwards: headings, bold, italic, strike, code and code blocks, nested lists with tasks, quotes, rules, links limited to `http`, `https` and `mailto`; HTML in a note is never run. Migration `00013` (`servers.note`, `note_updated_at`, `note_updated_by`); `GET /api/servers/{id}` carries the note, `PUT /api/servers/{id}/note` saves it (administrators; the audit gets `server.note` with the length, not the text). `Modal` got header actions and a separate `Esc` handler. Tests: the API against a real database (save, normalizing line ends, the length limit in characters, an unknown server, no text in the audit, operator and auditor read but cannot write) and `node --test web/src/lib/markdown.test.mjs` (escaping, unsafe links, quotes in attributes, bare addresses, code spans, blocks, odd input). Docs: `docs/ru/modules.md`, `docs/en/modules.md`.

- New: the "Add server" window has three tabs: **Confirmations** (the pending requests, the connected servers that are not on the dashboard, the connection details of the client), **Install by a link** and **Install over SSH**. Mockup: `docs/design/next/add-server.html`.
- **Install by a link.** The panel makes a link (lifetime, number of servers, an optional group, optional allowed networks, optional automatic approval) and shows two commands, an external one and a local one: `curl -fsSk --pinnedpubkey sha256//… 'https://host:port/v1/join/lk_…' | sudo sh`. The agent port serves the script and the binary of the cached, verified release to whoever has a valid token (`GET /v1/join/{token}` and `.../binary`), the key of the server is pinned in the command, and the script checks the binary against the signed manifest. `linager install agent` and `linager agent enroll` take `--join-token`. A request that came by a link needs no key: one click in "Confirmations", or none with automatic approval; the name of the server is the one the agent reports, the group is the one of the link. Only the hash of a token is stored (migration `00012`); a token can be limited to networks, to a number of uses and in time, and can be revoked; every step is in the audit journal (`join.create`, `join.revoke`, `enroll.join`, `enroll.approve`). API: `GET/POST /api/join-tokens`, `DELETE /api/join-tokens/{id}`; `GET /api/enrollments` has `via` and `needs_key`.
- **Install over SSH.** The panel connects to a new host with a password or a key, shows the host key to confirm, checks the system, systemd, disk, rights (asks for the `sudo` password when needed), that the host can reach the client port of the server, and an existing agent; then it installs the agent with a one-shot token that approves itself, shows the stages and the output, and rolls the host back (`agent uninstall --purge`, the server entry removed) when the agent does not connect. Secrets live only in memory for the length of the install and are never in the log or the audit; loopback, link-local and multicast targets are refused, `ssh_install.allowed_networks` in `server.yaml` narrows the range; a password is accepted over HTTPS only. API: `POST /api/ssh-install`, `GET/DELETE /api/ssh-install/{id}`, `POST .../answer`, `POST .../install`.
- Tests: the install links end to end against a real database and a real agent (valid, spent, revoked, limited-network and self-approving links), and the SSH installer against an in-process SSH server (host key, sudo with a wrong and a right password, an unreachable port, an unsupported system, a wrong password, a failed install with the rollback, targets and sessions, no secret in the state, the log or the audit). Docs: `docs/ru/install.md`, `docs/en/install.md`.

## 0.1.77

- Agent update: the panel shows the stages with their times instead of two states ("download" and "restart"). Why only two were seen: the server kept only the last step of the run and the panel asked every 2 seconds, so the stages that take milliseconds (verify, backup, replace) were never caught, and everything after the restart was one "restart" until the agent came back. Now the server keeps the timeline of the run (migration `00011`, `agent_updates.steps`) and the panel draws it as a list with marks, durations and details: the download with its size (`7.2 of 18.4 MB`), the signature check, the backup, the replacement with the rollback timer, the restart, and then the stages that the server sees by itself: **agent offline** (with the seconds left before the rollback), **the new version connected** and **confirmation**. The list is on the server page (the banner) and on the Updates page (the "Stages" button in the row of the agent); after an error or a rollback it stays and marks where it stopped. Only the current run is kept. In the agent: every stage sends an event when it begins and when it ends (`state`, `bytes`, `size` in `update.event`; agents of older versions keep working: their events are turned into the same list), the download reports its progress once a second, the new version sends a "confirmed" event after cancelling the rollback timer, and the binary is checked after it has been downloaded into a file that is not executable yet (it becomes executable only after the signature and the checksum are verified). The rollback delay (70 s) moved to `protocol.AgentRollbackDelay`. Tests: the stages of an old and of a new agent, the drop, hello, confirmation, rollback and the watchdog (database), and `Apply` end to end against a signed release, including a tampered binary.

## 0.1.76

- New: `linager` without arguments in a terminal opens an interactive menu (the logo, the state and numbered actions). The mode is found on its own: the server menu, the agent menu, a picker with two cards when the host has both, or an offer to install when there is neither. The state refreshes every 5 seconds. Server items: update, backup, restore (a list of archives), the administrator password, turning off the access restriction, the server fingerprint with the enrollment command, restart, logs. Agent items: enroll, update, restart, logs, remove. The items run the usual subcommands (through `sudo` for a user that is not root), dangerous ones need a confirmation, and the server actions are written to the audit journal as "CLI". Without a terminal `linager` still prints the help. Docs: `docs/ru/menu.md`, `docs/en/menu.md` (also on the site). Tests: detecting the roles, navigation, the confirmation, the password form, the dictionaries.
- New: `linager server reset-password --login L --password-file F` sets a new password for an existing user, lifts a sign-in lock-out and ends the user's sessions (until now `create-admin` worked only on an empty database). It writes an audit entry.

## 0.1.75

- Fix: on the dashboard the server group picker was lower than the filter buttons next to it. The items of the toolbar groups now stretch to the height of their row.

## 0.1.74

- Fix: the last line of the terminal window (the prompt) was cut off by the bottom edge. The terminal fits its rows to the height of the element it sits in, and that element had padding and a frame that were counted as free room. The frame and padding are now on an outer block and xterm sits in an inner one without them.

## 0.1.73

- Fix: a server update rolled back with "the new version did not become healthy: admin panel answered 403 Forbidden" whenever the admin access restriction was on and did not list 127.0.0.1 (or "only through trusted proxies" was ticked). The health probe of the updater (a plain GET of the panel root from the server itself, without `X-Forwarded-For` and without `text/html` in `Accept`) is now let through the restriction by the server; every other request is judged as before. The fix is on the server side because the update is run by the updater of the version being replaced, so it works for the update to this version too. Test: the guard lets the local probe in and refuses a browser, a proxied request, other paths, writes and other hosts.

## 0.1.72

- Admin access: the "Trusted proxies" and "Access journal" blocks are collapsed by default; "Access restriction" stays open.

## 0.1.71

- Server update, progress dialog: it no longer freezes on the first step and then ticks everything at once. The panel is served by the service that the update stops (from "Stopping the service" to "Start and check"), so for a few seconds it cannot learn the progress. The dialog now says so: the steps that had not finished at its last answer are shown as going on, with a seconds counter and an explanation, and when the server is back it shows the real result. The updater records when every step began and ended (`started_ms`, `finished_ms` in `update-status.json`), and the dialog shows how long each step took.

## 0.1.70

- Admin access: the white and black lists accept host names next to addresses, for a connection without a fixed public address (DDNS, for example `home.example.duckdns.org`). The server resolves the names once a minute and judges by the addresses they point to (IPv4 and IPv6); the list shows them with the time of the last check and a "Refresh" button. A name is added only when DNS knows it. When DNS stops answering, the last known address stays in force (it is stored and survives a restart) for 24 hours. A name that has no address blocks saving, like locking yourself out. Names are not allowed among the trusted proxies. New `POST /api/access/resolve`; `GET /api/access` and `POST /api/access/check` carry `hosts`.
- Tests: the restriction is in force from the first request after a start (a regression test).

## 0.1.69

- New settings section "Admin access" (Settings → Admin access): restricts who may open the admin panel. Three modes: off, white list (whatever is not listed is refused) and black list (whatever is not listed is allowed), a separate list for each, judged by the real client address. A checkbox "Only through trusted proxies" refuses direct connections to the port. "Check an address" shows what the server would do with an address, and the panel tells what happens to you before anything is saved. The restriction applies to the admin panel port only; the agent port is not affected.
- The trusted proxies (the addresses whose `X-Forwarded-For` is believed) are edited in the same section. The panel shows how the current request arrived and offers to add that proxy with one click. A connection from any other address is judged by its own address, so the local network can still connect directly while the internet comes through the reverse proxy. `admin.trusted_proxies` in `server.yaml` is used until something is saved in the section.
- Saving is refused when it would lock you out, when everybody is trusted (`0.0.0.0/0`), when a proxy that sends `X-Forwarded-For` is not in the list (otherwise the whole internet would look like the proxy) or when the white list is empty. If access is lost anyway, `linager access reset` on the server turns the restriction off (the lists stay); a running server applies it within seconds.
- Access journal: page opens, sign-ins (good and failed) and refused requests with the client address and the proxy it came through; filters, "to the white/black list" in one click, retention 7, 30 or 90 days. Changes of the settings are written to the audit log (`access.update`, `access.retention`, `access.reset`). New migration 00010.
- Settings: the sections are in a new order: Profile, Users, Server groups, Snippets, Admin access, Updates, Log.

## 0.1.68

- Updates, "Linager server": after a check that finds a newer release nothing pops up any more, the block just shows the button "Update to ...". The release notes open only by the button "Release notes" (without an update button in the dialog).
- The progress of a server update is a dialog now (it is gone from the block): it opens when the update is confirmed, lists the steps and the result, and after a good result it closes by itself in 5 seconds or by the OK button. After an error or a rollback it stays until OK. A dialog is also opened if a running update is seen after a page reload.

## 0.1.67

- Security: the "State" block is folded by default (its header shows the verdict, for example "4 things need attention"), and fail2ban moved to the first place, before "State" and "Now in the system". Same order on a phone.

## 0.1.66

- Updates, "Linager server": the notes of a release are shown in a dialog instead of a list inside the block. After a check that finds a newer release the dialog opens by itself (with a button to update right away), and the new button "Release notes" opens it any time. When the server is up to date the button shows the notes of the latest release. `GET /api/updates` now fills `server.notes` for the latest known release also when no update is available, and adds `server.notes_version`.

## 0.1.65

- Updates: the blocks "Agents" and "Same from the terminal" are folded by default, like "Update history". The header of "Agents" tells what needs doing: "to update: N" (the agents that can be updated, including failed and rolled-back ones), "all up to date", or "update running" while a rollout is in progress.

## 0.1.64

- Updates: the block "Update history" is folded by default (the header shows the number of records), like the other folded blocks.

## 0.1.63

- Phone, Firewall: "Ports" is no longer a tab of the segmented control but a folded block under the rules (closed by default, the header shows the number of listening ports), like on a desktop. The control keeps Rules, Lists and Log.
- Network (wide screen): the primary interface is a column of its own on the left, the other interfaces are a column of folded rows on the right (closed by default, a click unfolds a row in place). The badges (state, kind, primary) stand under the interface name. Under 900 px the columns stack.
- Side menu: the arrow of the fold button was redrawn as a clean symmetric pixel arrow (it was lopsided).

## 0.1.62

- Metrics: the blocks "Events in period" and "Metric retention" are folded by default (the header shows the number of events).
- Processes: new sort "By name" (all processes, alphabetically; "Show" gets 100 and "all", the agent caps a list at 2000). The rows show CPU and RAM as bars, the command as the executable name plus muted arguments, the state as a tag with a hint, and the end button is compact.
- Network: the primary interface comes first and is open, the others are one-line rows (name, state, kind, address, traffic) that unfold on a click, folded by default. DNS can be edited (new `POST /api/servers/{id}/network/dns`, audit entry `network.dns`): 1-3 name servers and up to 6 search domains. The change goes through the same confirm-or-roll-back window as the interfaces. The agent writes where the host keeps DNS: `/etc/resolv.conf` (atomic rewrite), a drop-in `/etc/systemd/resolved.conf.d/linager.conf` under systemd-resolved, or the `dns-*` lines of the primary stanza with resolvconf; netplan hosts stay view-only. `network.state` gets `dns_editable` and `dns_backend`.
- Firewall: two blocks like on the Metrics tab. "Rules" is always open, "Ports" (ports and connections, top remote addresses, how to read) is folded by default and loads only when opened.
- Security: "Now in the system", fail2ban and "State" stay open, the other blocks (failed attempts, logins, keys) are folded by default. "State" became a checklist: a verdict ("Everything is fine" / "N things need attention"), one row per check with a mark, the value and, for every check that is not fine, a hint what to do. Phone: the accordions follow the same order.
- Docker: the agent checks the images of its containers by itself, 60 seconds after start and then every 6 hours (the button uses the same code). The number of containers with a newer image (`image_updates` in the sample, -1 = unknown) is a tag "images: N" on the server card (warn from 5) and a part of the summary hint under the updates figure. A new shared component `Fold` is used for the folded blocks.

## 0.1.61

- Overview: the four bars (CPU, RAM, network) are filled at once when the page opens. `GET /api/servers/{id}` now returns `recent`, the last 60 raw samples (CPU, RAM, receive and send rate) from the stored metrics, and the page seeds its bars from them instead of starting empty and collecting samples live. RAM and network were empty before, CPU had only 12 values.

## 0.1.60

- Phone: the line with the number of servers above the summary is gone (the summary strip already shows it).

## 0.1.59

- Phone: the buttons in the header of a terminal window (user, snippets, keys, copy, paste) stand next to each other on the left instead of being spread over the whole width.
- Phone: with the keyboard open, the terminal (the tab and the floating windows) no longer shows the page through the gap between its key row and the keyboard. That strip, where the translucent keyboard and its accessory bar of newer iOS sit, is filled with a solid background behind the terminal.

## 0.1.58

- Phone: the lightning bolt of the snippets button was redrawn (a clean pixel bolt). Everything at the bottom edge (the tab bar, the button bars of dialogs, the dock of terminal windows) is lifted by at least 16 px above the rounded screen corners and the bars keep a margin at their sides; the fill of the bars still reaches the edge.
- Server groups moved to the settings (new section "Server groups", administrator only): create, rename, delete and, new, arrange the groups with the up and down buttons. The "Groups..." button of the dashboard is gone, on a phone and on a desktop. The order of the groups is kept on the server (migration 00009, `PUT /api/groups/order`, audit entry `group.reorder`) and is the order of the group list on the dashboard and in the access settings.

## 0.1.57

- Phone: while a layer is open (the terminal, a floating window, a dialog, a sheet) a finger move that cannot scroll anything inside it is cancelled, so the page no longer moves or bounces behind it, and the visual area does not pan while the keyboard is up. The content of a dialog and the list of a sheet still scroll.

## 0.1.56

- Server page: a square icon button at the right end of the "Terminal" item of the side menu opens the terminal in a floating window (shown while the server is online and the terminal module is allowed).
- Phone layout, fixes after the first real use: the page behind a full-screen layer (floating terminal window, dialog, sheet, drop-down list) no longer scrolls with the finger; the Terminal tab has no "open in window" button on a phone (a window stacked a second terminal over the tab); the button bar at the end of a dialog sits exactly on the bottom edge of the sheet; no gaps under the pinned bars (firewall and network draft/rollback, apt install, selection bar of Files, search bars of Metrics and Services, the toolbar of the dashboard); the dock of minimized windows no longer hides the last row of a page.
- Phone: "Terminal as user" in the action sheet of a user works again; the group picker moved out of the pinned row of the dashboard; map tiles lay their meters out as rows; the sign-in page no longer scrolls by 1 px; chart labels no longer overlap in English; fields of the terminal use 16 px.

## 0.1.55

- Phone layout, redone from scratch (up to 720 px, designed for an iPhone 14 Pro Max, mockup `docs/design/next/mobile.html`): a pinned top bar and a bottom bar instead of the drop-down menu (Servers, Linager updates, Log, More; on a server page Overview, Terminal, Files, Services and "Sections" with every tab by group). Drop-down lists, action menus, context menus and all dialogs open as sheets from the bottom edge, the buttons at the end of a dialog stay in reach, and the large dialogs fill the screen. No swipes; controls are 44 px high, fields use 16 px so iOS does not zoom.
- The viewport no longer scales the page to 0.9 and covers the notch and the home indicator (`viewport-fit=cover`, safe-area insets).
- Dashboard on a phone: the four tiles are one strip, the filters, search and the card/map switch are one pinned row, a card is shorter (no chart) and opens the server by a tap, its terminal button opens the Terminal tab. Edit mode has 44 px buttons.
- Server page on a phone: one header (back, section, server and state, actions), the frame around the tab is gone; every tab was adapted: tables became rows with an action sheet, the terminal and the logs fill the whole height with a row of special keys above the keyboard, the folder tree of Files is a sheet, the draft/apply/rollback bar of the firewall and the network is pinned above the bottom bar, the settings window starts with a list of sections.
- Terminal: the floating window opens 20% larger by default (864 x 528 instead of 720 x 440); a size you set yourself is kept.

## 0.1.54

- Theme: the panel no longer follows the light or dark setting of the system. The theme you pick with the button in the top bar is kept in the browser and shown after every reload; until you pick one it is dark. The colour of the browser bar on phones follows the chosen theme as well. Server only, agents need no update.
- Users: the tab shows all accounts by default (the "Regular" and "System" buttons narrow the list when pressed). The "Add user" window has two columns, so it fits a window of 720 px in height even with many groups, and the hint of a password option is shown only for the chosen one.
- Terminal: a "User" drop-down in the toolbar of every terminal window, like the one of the snippets: it lists root and the accounts that have a shell (read from the Users module, the chosen one is ticked) and starts the shell again as the picked account. The title of the window follows (`user@server`). The button is shown to administrators and to operators who were given the Users module. Needs agent 0.1.53 or newer.

## 0.1.53

- Users (new agent module `users`, tab "Users" in the group "Network and security"): the local accounts of a server. A table of the regular accounts (UID 1000 and up) and, on request, the system ones: rights (sudo, sudo without a password, superuser), groups (`sudo` and `docker` are marked as almost root), shell, last login with the address, sessions now. A row opens the properties window: sudo (none / with a password / without a password), the groups the panel offers, the shell, a lock of the account (an expired account, so neither a password nor a key works, and the sessions are ended). Add: login, description, shell, home, a generated or typed password (the browser makes it, it is shown once and goes to `chpasswd` through stdin), sudo, groups, an SSH key, with the commands shown before they run. Menu: change the password, end the sessions, delete (with the home directory or without, a typed name to confirm, the sessions and processes are ended first). Root and system accounts are never changed; the only unlocked user with sudo cannot lose the rights or be locked or deleted while root cannot log in. Every change asks for confirmation and goes to the audit log (`hostuser.*`, the password is not logged). It is a root module: an operator sees the tab only when the administrator gave them "Users".
- Terminal as a user: the "Terminal" button of a row opens a floating terminal window as that account (`runuser -l`, no password needed; the agent checks the name against `/etc/passwd`). The window has the same frame as the others (drag, resize, minimize, maximize), one window per user. It needs the Terminal module too. Needs agent 0.1.53; the tab is hidden for older agents.
## 0.1.52

- Server page: the tabs moved from the top row to a sidebar on the left, split into groups: General (Overview, Metrics, Terminal), System (Services, Processes, Logs, Updates), File system (Files, Disk), Network and security (Network, Firewall, Security) and Applications (Docker). Every tab has a pixel icon; a group is folded with a click on its title (the group of the open tab stays open). Counters: the number of apt updates on "Updates" (red when some of them are security updates) and of failed services on "Services". The button at the bottom folds the menu into a narrow column of icons (a hint shows the name on hover). The state is kept in the browser. A group whose tabs are all hidden (a disabled agent module, an operator without the module) disappears. Up and Down move between the tabs. On a narrow screen the menu is a drop-down with the current section on top. The page shows the group and the name of the open tab above the panel. Addresses `#srv-<name>/<tab>` are unchanged. Server only, agents need no update.
## 0.1.51

- Releases: `manifest.json` now has an `assets` map with the binary of every architecture (name, SHA-256, size); the top-level `sha256` and `size` still describe the amd64 binary, so agents of earlier releases keep updating. The server downloads, verifies and caches every binary of a release and gives an agent the one for its own architecture (`/v1/release/<ver>/binary?arch=`; an agent that sends none gets amd64). The agent checks the downloaded file against its own entry of the signed manifest. First step of arm64 support for agents.
- Build and release: `make build` produces `linager_linux_amd64` and `linager_linux_arm64` (`ARCHES=amd64` builds one), a release carries both binaries and one `.deb` per architecture (`linager_amd64.deb`, `linager_arm64.deb`), `SHA256SUMS` lists all of them and `manifest.json` has a checksum for each. CI runs the tests on an arm64 runner as well. 
- Installer: `install.sh` now accepts arm64 (a Raspberry Pi 4 with Raspberry Pi OS 64-bit, Debian or Ubuntu for arm64) and downloads the binary and the checksum of its own architecture. It refuses other CPUs and a 32-bit system on a 64-bit kernel (Raspberry Pi OS 32-bit) with a clear message. The "CPU" line of an arm64 agent shows the board ("Raspberry Pi 4 Model B Rev 1.5") instead of staying empty.

## 0.1.50

- Roles: besides the administrator there are now an Operator and an Auditor. When the administrator creates a user (or changes one in "Settings → Users → Role and access") they pick the role and, with two buttons, the servers and the modules. In the "Servers" window the servers are grouped; the switch of a group gives access to all its servers (they turn on and grey out) and to the servers that join the group later. The "Modules" window has a switch per module; Terminal, Files, Network, Firewall, Security and Docker are marked "root access", because they let a user do almost anything on the host.
- An operator works only on the servers and with the modules that were given, on every server of theirs; a closed module is closed as a whole. An operator cannot change the settings of the panel (users, updates, audit log, snippets, groups), add or delete agents, reboot servers or edit the dashboard. An auditor only sees the dashboard, the overview and the metrics of the chosen servers and changes nothing. Both get a "Profile" button instead of "Settings" (password, 2FA, passkey).
- The server checks every request: each route has an access rule (a test makes sure none is missing), a server the user may not see answers as if it did not exist, and the live events are filtered the same way. A change of rights works at once. The administrator cannot change their own role or demote the last administrator; creating a user and changing the access are written to the audit log. Existing users stay administrators. Server only, agents need no update.

## 0.1.49

- Agent: `/etc/linager/agent.yaml` is completed when the agent starts (so after every update): the modules a newer agent knows and the file does not list are appended to the `modules:` block as `true`, the rest of the file (comments, your `false` values) stays as it is. A module missing from the file was enabled before too, so nothing changes in behaviour; the file now lists everything. Needs agent 0.1.49.

## 0.1.48

- Firewall: a "Journal" button next to "Address lists" opens the journal of rule matches. A rule is marked for the journal with a switch in the table of rules (or "Switch on for all drop rules" in the window): the rule gets `limit rate 5/second log prefix "lg:..."` and the kernel writes the matched packets to its journal, which the agent reads (7 days). The window shows the matches over time and a table (time, chain, action, protocol, source, port, rule, packets) with filters by chain, action and protocol in any combination, by port (`22`, `80, 443`, `1000-2000`) and by source (an address, a network or `@list`), identical events can be grouped, a row expands to the details, and the whole list can be exported to CSV. From a row: add the source to an address list, block the address (a rule first in input, in the draft) or open the rule. Needs agent 0.1.48 and `journalctl`.

## 0.1.47

- Firewall: the rule `ct state established,related accept` is now written by the agent first in every chain (input, forward, output) and cannot be removed or disabled, so the replies of the connections that already exist (the agent link, an open SSH session, the replies to what the host opened itself) are never cut by a drop policy or a drop rule. The rule is not shown in the list (a note under the table says so) and is skipped when it comes from an older draft or from the nft syntax form. The warning before applying no longer asks for it, but still catches a rule that drops all TCP before the SSH port.
- Firewall: the rules are shown as a table with the columns Action (Allow, Drop, Reject), Protocol, Port, Source (an address, a network, `@list` or an interface) and Comment (with the conditions that have no column, such as the connection state or a rate limit); the new "All" tab shows the rules of all chains with a Chain column and the "As nft" button switches to the raw rule text. The automatic rule needs agent 0.1.47; with an older agent the list is already shown as a table.

## 0.1.46

- Firewall: the Source field of a rule (and of the "Restrict" form for a port) offers the address lists in a drop-down when it gets the focus: it narrows as you type, a click or the arrow keys and Enter put `@name` into the field. Server only, agents need no update.

## 0.1.45

- Disk tab (new agent module `disk`, right after Files): every partition as a pie chart with the free and the used space, the growth per day and a forecast "fills up in ~N days" (the agent records the used space every half hour); a measurement on request shows what takes the space (system, Docker, logs, databases, cache, home directories) and the biggest directories with drill-down; cleanup: journald, apt cache, unused Docker images, Docker build cache, old kernels, each with a confirmation and an audit entry. The pie charts share one look: a sector lifts on hover, a tooltip shows the size and the legend row is highlighted.
- Security: fail2ban can be installed from the panel (three strictness modes, the address of the administrator is never blocked) and configured (threshold, ban time, repeat offenders, "Do not block" list); a block list with the history of bans, unban of one, selected or all addresses, manual blocking for a chosen time and a "Ban" button in the table of failed attempts. A key can be removed from `authorized_keys` (a copy of the file is saved first, a warning for the last key). Needs agent 0.1.45.
- Firewall: the chains are tabs; a "Ports and connections" block with the listening ports, processes, connections and the state of each port in the firewall (with "Close the port" and "Restrict" that add a rule to the draft); "Address lists": named lists of addresses (nftables sets) that rules use as `@name`. Needs agent 0.1.45.
- Docker: "Check images" compares the digest of the tag in the registry with the image on the host (no download) and marks containers with a newer image; "Update marked"; compose containers are grouped by project (collapsed by default) with "Update project", "Restart", "Start project" and "Project logs". Needs agent 0.1.45.
- Heights of buttons, inputs and selects in a row are the same everywhere (the header "Settings" button with a counter, the dashboard search, the add-passkey row, the add-rule row of the firewall).

## 0.1.44

- Security tab: the "Logged in now" and fail2ban blocks moved right under the "State" block. Server only, agents need no update.

## 0.1.43

- Security: remote SSH sessions can be ended from the "Logged in now" block: the "End" button of a session and "End all sessions" for a user with several. A confirmation warns that the processes of the session stop too (tmux, screen, long operations) and that this is not a block. The agent ends only sessions from its own list of remote sessions (`loginctl terminate-session`), local console sessions are never touched, and every action goes to the audit log. Needs agent 0.1.43 and systemd-logind.

## 0.1.42

- Security tab (new agent module `security`, read-only): the SSH state (port, root login, password and key login, empty passwords, user restrictions), failed sign-in attempts of the last 7 days (total, by day, top addresses and logins, probes of users that do not exist), SSH logins (a login from an address that guessed passwords earlier is flagged), fail2ban jails and banned addresses, automatic security updates, the keys of every `authorized_keys` (fingerprints only, weak RSA/DSA marked) and the remote sessions now. Data comes from the systemd journal (`sshd` and `sshd-session`, so Debian 13 works), with `/var/log/auth.log` as a fallback. Needs agent 0.1.42; the tab is hidden for older agents.

## 0.1.41

- Settings: the window is wider (up to 1360 px). Server only, agents need no update.

## 0.1.40

- Settings: a "Settings" button in the top bar opens a large window with a side panel: Profile, Users, Updates, Audit log, Snippets. The old pages #updates and #audit moved into it (the old links still work); the 2FA button is now part of the profile.
- Sign-in: a "Passkey" button next to "Sign in" signs in without a login and a password (WebAuthn, needs HTTPS or localhost). Passkeys are added in the profile. "Sign in" goes to the 2FA step as before; the code field also takes a recovery code.
- Recovery codes: enabling 2FA shows 10 one-time codes (copy or download, shown once); the profile shows how many are left and can generate new ones (password required).
- Users: add users, change your own password (the other sessions end), reset the password or the 2FA of another user, delete users (not yourself). All actions go to the audit log. New `admin.login_rate` in server.yaml (sign-in attempts per minute per address, default 10).
- Snippets: commands shared by all users, managed in Settings. In the terminal (tab and floating window) a "Snippets" drop-down: a click pastes and runs the command, the small button at the right only pastes it.
- Docker: "Update image" in the container menu pulls the image again and recreates the container with the same settings, with a rollback to the old container if the new one does not start; a docker compose container is updated with `docker compose pull` and `up -d --no-deps`. Needs agent 0.1.40.

## 0.1.39

- Sign-in page: the card is centered under the top bar. Two-step sign-in: login and password first, then the 2FA code on a second step (only for users with 2FA; no more "I have 2FA" checkbox). The right password without a code creates no session and is not counted as a failed attempt. The code signs you in by itself when the sixth digit is typed or filled in. The fields are marked for password managers (`username`, `current-password`, `one-time-code`). Needs the server only; agents need no update.

## 0.1.38

- Metrics: a "Synchronize" checkbox next to the period chips shows the hover highlight (crosshair, dots and tooltip) on all four charts at once; the choice is remembered in the browser, off by default.
- Terminal: the keyboard icon in the key panel button is centered.

## 0.1.37

- Updates: after a successful install the progress bar is filled completely (apt's last reported percentage stays below 100, so the last segment stayed empty). Server only, agents need no update.

## 0.1.36

- Dashboard: the number of updates on a server card (and the security count) is now counted the same way as on the Updates tab. Before, the dashboard missed packages that need a new package to be installed, such as kernel updates, so a server could show 2 on the card and 4 on the tab. Needs agent 0.1.36; the card refreshes within an hour or after an apt run from the panel.

## 0.1.35

- Updates: installing packages now shows the state of every package (queued, updating, done, error; "not updated" for packages apt never reached after a failure) and an overall progress bar (download, then install). "Check" opens a progress dialog that closes by itself three seconds after a successful check. The apt console output is hidden by default behind a "Show console output" button. The statuses come from `APT::Status-Fd` on the agent, so they survive closing the tab: the browser attaches to the running operation and gets all events and the log. Needs agent 0.1.35.

## 0.1.34

- Terminal: the shell now gets HOME, USER, LOGNAME and SHELL (the agent service had no HOME, so `cd` said "HOME not set" and bash did not save its history). Commands are appended to the history right after they run, and a closed session gets three seconds to exit by itself before it is killed. Needs agent 0.1.34; open terminal sessions pick it up after the agent restarts.

## 0.1.33

- File editor: a "Wrap lines" switch in the window header wraps long lines instead of scrolling sideways. The choice is remembered; it is on by default on touch screens.

## 0.1.32

- Phones: the terminal buttons (keys, copy, paste, in a window, close session) are pixel icons instead of text; the floating terminal window shows copy and paste as icons.

## 0.1.31

- Terminal key panel redone: no scrolling, a thin strip of five modes (Main, Ctrl+, Nav, F1-12, Symbols) and the arrows on the right like on a real keyboard (up above down), with Esc and Tab beside them. Ctrl and Alt still latch until the next key.

## 0.1.30

- File manager: "Properties" in the context menu (also for the current folder): path, type, size (folder content size on demand), creation, modification, change and access times, links and inode, link target; permissions (rwx checkboxes, setuid, setgid, sticky and the octal number) and owner and group can be changed, with a confirmation for recursion, special bits and owner changes. Needs agent 0.1.30.
- Terminal: a key panel for phones behind the keyboard button (Ctrl and Alt latch, Esc, Tab, arrows, Home, End, PgUp, PgDn, Ins, Del, F1-F12, symbols and common Ctrl shortcuts).

## 0.1.29

- Metrics: the "Events for the period" list scrolls inside its block (up to 320 px) instead of stretching it.

## 0.1.28

- Docker tab: the container and image actions moved into a drop-down "Actions" menu in each row.

## 0.1.27

- Phones: the agents, cache and history tables on the Updates page are shown as compact blocks, so there is no horizontal scrolling.

## 0.1.26

- Phones: the four metric cards on the server overview are smaller and sit two by two.
- Phones: the lists on the Services, Processes, Updates and Docker tabs are shown as compact blocks instead of wide tables, so there is no horizontal scrolling in them.

## 0.1.25

- Dashboard map: bigger tiles with the address, three metrics with mini bars (CPU, RAM, disk), network with coloured arrows, uptime and an event badge (also "no issues"); six tiles per row on wide screens; long press opens the menu on touch screens.
- Dashboard map menu: "Delete server" removes the server from the admin panel (with the usual confirmation); choosing the group is available only in edit mode.
- Dashboard cards: the buttons are pixel icons (terminal, open) plus a trash button that deletes the server from the admin panel (with confirmation).
- Updates: "Delete old" removes cached releases except the newest one and the version the server runs (refused while an update or rollout is running).

## 0.1.24

- Dashboard: only the server list scrolls, the summary and the filters stay in place (screens wider than 720 px).
- Dashboard: the "Rows" view is replaced by the "Map": small tiles with a status-coloured edge, name and CPU/RAM. Left click opens the server; right click (long press on touch screens) opens a menu: Terminal, Open, Group, Remove from the dashboard.

## 0.1.23

- File manager: the number of selected items is shown to the right of the path instead of a separate line above the list.

## 0.1.22

- File manager: right-click context menu (open, download, cut, copy, paste, rename, delete; on the empty area: paste, new folder, new file, refresh). The row buttons are gone; touch screens get a "⋯" button in the row.
- Multiple selection with Ctrl/⌘+click and Shift+click (also Shift+arrows, Ctrl/⌘+A), and the keys Ctrl/⌘+C, X, V, Delete, F2. Several files and folders download as one zip.
- Paste asks "Overwrite" or "Keep both" when a name already exists, with a "For all" checkbox when there are several conflicts.
- Agents must be updated to 0.1.22 for rename, copy, move and zip.

## 0.1.21

- Files: uploading a file no longer fails with "target is a directory" (the panel sent the folder instead of the full target path). Only the panel changed, agents need no update.

## 0.1.20

- Network tab on netplan systems (Ubuntu): the configuration can be edited by hand. Pick a file from `/etc/netplan` (or create a new one); it is checked with `netplan generate` and applied with `netplan apply` under the same 30-second automatic rollback. cloud-init generated files get a warning, and "Effective configuration" shows the merged result (`netplan get`).
- Services: the actions moved into a menu. New "Ignore" item: an ignored failed service no longer counts towards the server status and the dashboard counters.
- Terminal tab: the "In a window" button moves the terminal into a floating window.
- File editor: opens in floating windows like the terminals (move, resize, minimize, several at once) and uses CodeMirror 6 with syntax highlighting (PHP, HTML, CSS, JavaScript, JSON, YAML, XML, SQL, Markdown, Python, shell, nginx, Dockerfile, TOML, ini and more), line numbers and search.
- Agents must be updated to 0.1.20 for netplan editing and for ignoring services. Database migration 00006 adds the ignore list.

## 0.1.19

- Network tab: the configuration block can be edited by hand (ifupdown systems): the whole `/etc/network/interfaces` is edited, checked with `ifup --no-act`, applied with the same 30-second rollback as form changes, and written to the audit log. Netplan systems stay read-only. Agents must be updated to this version to use it.

## 0.1.18

- Server groups: create groups ("Groups…" next to the dashboard filters), put a server into a group in edit mode, and pick a group in the selector to see only its servers (the summary follows the selected group). Reordering works inside the selected group. Group changes are written to the audit log. Database migration 00005 adds the groups table.

## 0.1.17

- Server overview: the interfaces table takes the full width with aligned columns; the system block moved below it and shows its rows in several columns.

## 0.1.16

- The pairing ("Adding a server") dialog has a Back button that returns to the "Add server" dialog.

## 0.1.15

- Docker tab: the container buttons stand in one line (start and stop are one button that follows the state), so rows are much lower; on screens up to 1000 px the buttons wrap into two lines and ports and RAM are hidden.

## 0.1.14

- "Add server" dialog: client connection details (address, port, certificate fingerprint) with a button that copies `address:port`. The address is the one the panel was opened with, so it is the domain when the panel is behind a reverse proxy.
- Docs: Nginx Proxy Manager settings; the live-updates (SSE) note rewritten in plain words.

## 0.1.13

- Server page: no more gap between the active tab and the panel below it.

## 0.1.12

- Phones: the top bar is compact (logo, live indicator and a menu button) and stays pinned while the page scrolls; the buttons open as a panel under it.
- Checkboxes are restyled to match the rest of the interface (square with a hard edge and offset shade, pixel tick).
- The logo in the top bar is now the same `>_` pixel icon as the home screen icon, and the icon no longer has the extra bottom square.

## 0.1.11

- Phones: the admin panel is shown at 90% scale.
- Home screen icon (pixel console `>_`) and a web app manifest: a page added to the phone's home screen gets the icon and opens without the browser address bar.

## 0.1.10

- Phones: the admin panel no longer zooms (pinch, double tap and focusing an input field).

## 0.1.9

- The top bar stays pinned to the top of the window while the page scrolls (on screens wider than 720 px).

## 0.1.8

- Dashboard rows view: the "Terminal" and "Open" buttons stand one above the other to the right of the metrics.

## 0.1.7

- Files tab: the tree and the list fill the window height and scroll inside (the table header stays visible) instead of
  making the page grow.

## 0.1.6

- The "Linager updates" page no longer freezes after "Check" finds a release without notes: the server always sends a
  list (never null) and the page tolerates missing fields.
- Release notes on GitHub are taken from this file, so the updates page shows the real list of changes; wrapped lines
  of one item are joined.

## 0.1.5

- Metrics charts are redrawn: the time axis covers the whole selected period, averages are lines with a light fill and
  peaks a dashed line, gaps break the line, round y-axis steps, events as markers under the axis, legend and a hover
  tooltip with values and events. Network receive and send use two colour-blind-safe colours.
- Services table: rows keep one height and the action buttons stay on one line.
- Server page: the Processes tab follows Services; the page is centred.

## 0.1.4

- The "Linager updates" page no longer hangs on "Loading…" while a release check runs: the check talks to GitHub
  without holding the updates lock, and repeated clicks on "Check" share one request.

## 0.1.3

- Files tab works like a file explorer: a folder tree on the left (lazy loading, resizable), the folder contents on the right;
  click selects, double click or Enter opens a folder or the file editor; binary and large files offer a download.
- The server name follows the hostname: the agent reports it on every connection (also after a reboot) and right after it
  changes; a rename is recorded as a server event. Names that were not derived from the hostname stay as they are.
- All drop-down lists use a project-styled control instead of the native one.

## 0.1.2

- Dashboard: the "Terminal" button opens a floating terminal window over the dashboard. Windows can be moved, resized,
  minimized to a bar at the bottom, maximized and closed; several can be open at once (up to 3 per server).
- When the per-server terminal limit is reached, the interface shows a notice (the server now reports it as WebSocket
  close code 4029).

## 0.1.1

Fixes after the 0.1 review.

- New server setting `agents.cert_lifetime` (default one year).

- Agent certificates renew themselves over the channel before they expire (previously every agent would have dropped off after one year).
- Network and firewall changes get a second, out-of-process rollback timer for the case when the agent itself crashes or hangs.
- apt operations keep running when the browser tab closes or the connection drops; reopening the tab shows the log from the start.
- The SSH lock-out warning uses the ports sshd really listens on, not only port 22.
- The agent reconnects quickly again after a long healthy session (the pause used to stay at its maximum).
- `curl … | sudo sh` now starts the interactive wizard.
- Server update: the database is restored automatically when an update fails after its migrations; the root updater no longer trusts
  files the server user can change; downgrades are refused; new `linager restore` command.
- Agent rollouts survive a server restart; a failed first agent no longer stalls a "first, then the rest" rollout.
- Interface: dotted shape of large headings as in the mockup, working "Terminal" button on dashboard cards, agent update tags and banner,
  tabs of disabled modules are hidden.

## 0.1.0

First release.

- Server and agent in one Go binary; PostgreSQL; embedded Svelte web interface (ru/en, light/dark).
- Agent pairing with fingerprint pinning, check code and enrollment key; mTLS and signed messages afterwards.
- Dashboard (cards and rows, drag and drop order), live metrics (5 s) and history up to 360 days with roll-ups, events and CSV export.
- Web terminal, systemd services, apt updates, processes, journal, files, Docker.
- Network (ifupdown/ifupdown2) and nftables firewall with drafts and a 30-second automatic rollback enforced by the agent. Netplan: view only.
- Installer (interactive TUI and non-interactive): PostgreSQL, Caddy (Let's Encrypt) or own reverse proxy, systemd units.
- Signed self-update of server and agents with backup, health check and automatic rollback; rollout modes and offline queue.
- Backups (database, configuration, keys), audit log, optional mandatory 2FA, security hardening.
