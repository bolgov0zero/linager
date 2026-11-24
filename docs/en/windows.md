# Windows agent

The agent runs on Windows 10 / 11 and Windows Server 2016 and newer (x64) and connects to a Linager server the same way as the Linux one. The **server stays on Linux**. The Windows agent has the modules that map onto Windows directly, and nothing else.

## What works

| Tab | What it does on Windows |
|---|---|
| Metrics | CPU, memory, swap, disks (every volume with a file system), network adapters and their speed, uptime, processes, "restart required" (Windows Update). Windows has no load average: it is shown as 0. |
| Terminal | PowerShell (cmd.exe if it is missing) in a pseudo console (ConPTY, Windows 10 1809 / Server 2019 and newer). The shell runs as **SYSTEM**, the full access of the agent, like the root shell on Linux. Everything is written to the audit journal. |
| RDP | the screen of the computer in the browser (RDP through the agent), see "RDP" below. |
| Assistant | the screen of the console or of another RDP session without a new session, see "Assistant and Management" below. |
| Services | the list from the Service Control Manager: start, stop, restart, startup type (enable = automatic, disable = disabled). The agent itself, remote desktop, WinRM, sshd, the RPC, DHCP, DNS and firewall services ask for confirmation. A service that stopped with an error and starts automatically counts as failed. |
| Processes | the list with CPU, memory, user and command line; "end process" (Windows has one way to end a process, so "terminate" and "kill" do the same). System processes (csrss, lsass, services, wininit, winlogon, smss, System) are refused. |
| Logs | Windows event logs, followed live: System and Application by default; as "unit" a classic log (`Security`, `Setup`), an operational log (`Microsoft-Windows-TaskScheduler/Operational`) or the name of a source (for example `Service Control Manager`). "Kernel" shows the kernel sources. The priority is mapped to the level (error, warning, information). |
| Files | the file manager. The top of the tree is the list of drives, paths look like `/C:/Users/x`. Owner and group are not shown, and permissions cannot be changed from the panel (Windows keeps ACLs). The drive roots and `C:\Windows`, `C:\Program Files`, `C:\ProgramData`, `C:\Users` cannot be renamed, deleted or replaced. |
| Reboot | `shutdown /r`, with confirmation. |

Not available on Windows (the tabs are hidden): firewall, security (fail2ban, keys), Docker. Users, updates and network have their own Windows views, see below.

## Install

Download `linager-agent_windows_amd64.msi` from the release (check it against `SHA256SUMS`) and install it in an elevated prompt:

```powershell
msiexec /i linager-agent_windows_amd64.msi /qn SERVER=panel.example.com:9443
```

`SERVER` is the agent port of the server (`host:port`). `FINGERPRINT=SHA256:...` is optional: with it the agent trusts only that server certificate; without it the pairing is protected by the check code (see below). The package installs `C:\Program Files\Linager\linager.exe`, registers the service **Linager Agent** (`LinagerAgent`, automatic start, runs as SYSTEM, started again after a failure) and starts it.

### Install by a link (one command)
The easiest way: in the panel open "Add server", tab "Install by a link", create a link and switch the command to **Windows**. On the new computer open PowerShell **as administrator** and paste the command. It checks the certificate of your server by its fingerprint (carried in the command), downloads the installer from the server, compares it with the signed release, hands the one-time code of the link to the service through a file that only SYSTEM and the administrators can read (`C:\ProgramData\Linager\agent\join.token`), installs and starts the service. The computer appears in the "Confirmations" tab and is confirmed in one click: no key to copy and no file to open. The script can be read first at `https://<server>/v1/join/<link>/windows`.

### Install from a local file
Download `linager-agent_windows_amd64.msi` and `SHA256SUMS` from the release (or take the file from the server: `https://<server>/v1/join/<link>/msi`), compare the checksum (`Get-FileHash -Algorithm SHA256 .\linager-agent_windows_amd64.msi`), copy the MSI to the computer and, in PowerShell as administrator:

```powershell
msiexec /i C:\path\linager-agent_windows_amd64.msi /qn SERVER=host:port FINGERPRINT=SHA256:...
```

The address is the agent port of the server, the fingerprint is shown in the panel (the install link command carries it). Then confirm the computer in "Confirmations" with the key from `C:\ProgramData\Linager\agent\enroll.txt`. For the confirmation in one click without a key, create a link in the panel and write its code to the protected file before the installation:

```powershell
New-Item -ItemType Directory -Force C:\ProgramData\Linager\agent | Out-Null
icacls C:\ProgramData\Linager /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F'
Set-Content C:\ProgramData\Linager\agent\join.token 'lk_...' -NoNewline -Encoding ASCII
```

### Pairing
1. In the panel open "Add server", tab "Confirmations": the host appears there a few seconds after the service starts.
2. On the Windows host read the key and the check code (administrators only):

   ```powershell
   Get-Content C:\ProgramData\Linager\agent\enroll.txt
   ```

3. Enter the key ("Confirmation key") and **make sure the check code is the same**: it ties the key to the certificate the agent saw, so it protects the pairing against a man in the middle when no fingerprint was given. Confirm.

The same works from a console: `& "C:\Program Files\Linager\linager.exe" agent enroll --server panel.example.com:9443`. The service notices the new identity within 10 seconds.

## Users, updates and network
These tabs work on Windows too, with their own views (no shells or sudo): **Users** (local accounts and groups), **Updates** (Windows Update), **Programs** (installed programs) and **Network** (adapters).

- **Users.** The list of local accounts: name, full name, enabled or not, last sign-in, groups. Add a user (a password is required: Windows lets nobody in over the network without one), set a password, enable or disable, change the groups (for example Administrators or Remote Desktop Users), delete. The sessions that are signed in now (the console and remote desktops) with "End session". Guards: the built-in administrator (the account whose SID ends in -500) is not deleted or disabled from here, and the last enabled administrator is not removed or disabled. Domain accounts are not managed. Every change is confirmed and written to the audit log (`win.users.*`, without passwords).
- **Updates.** The search of Windows Update on the computer itself (it can take a minute; where the computer uses WSUS the answer comes from there): title, KB, size, severity, "security". Install the selected or all updates with a live log (download, installation, result codes, whether a restart is needed). The dashboard shows the number of updates and "restart required" like it does for apt; the agent searches once an hour, the first time 3 minutes after the start. Programs of other makers (winget) are not covered.
- **Programs.** A view-only list of installed programs: name, version, publisher, install date, size and for whom it is installed (the whole computer with x64/x86, or one user). The agent reads the Uninstall keys of the registry (the computer in the 64 and 32 bit views and the loaded user profiles); it starts and removes nothing. System components, Windows updates (KB) and entries without a name are left out, a program seen in both registry views is shown once. There is a search by name and publisher and sorting by column. The module is `programs` (`modules: programs: true|false` in `agent.yaml`), an operator needs the `programs` right. With an old agent the tab asks to update the agent.
- **Network.** The adapters: state, DHCP or static, IPv4 addresses, gateway, DNS, MAC, speed, MTU. Change one adapter (DHCP, or an address with the prefix and the gateway, and the DNS servers). **The agent undoes a change by itself after 30 seconds unless you confirm it**: it saves the old settings and creates a one-shot task of the Task Scheduler (`LinagerNetRollback`), so the rollback works even if the connection is lost. A change left unconfirmed by a restart is undone when the agent starts. Only IPv4; no Wi-Fi, VLANs or teaming. A change is refused while another one waits for its confirmation. The audit log has `win.network.apply` and `win.network.rollback`.

## Disk
The "Disk" tab is the same as for Linux: the volumes with the used and free space and a forecast of when the space runs out (from a history the agent keeps itself, a point every half hour), what takes the space on a volume (a tree of folders with sizes and a pie: system and programs, user profiles, temporary files, other, free; the scan can take a minute and is marked partial when it did not finish), and the ways to free space. Each way is confirmed, shows the size in advance and is written to the audit log: temporary files of the system and the users, the downloads of Windows Update (the update service stops for the moment), the Recycle Bin, the Delivery Optimization cache, crash dumps and error reports, `Windows.old`. Nothing else is touched.

## RDP
The "RDP" tab of a Windows server (and "RDP" in the context menu of a tile on the "Grid") shows the screen of the computer and lets you work with it, in the page or in a floating window like the terminal. Nothing is installed: the RDP client is part of the panel (WebAssembly, loaded when the desktop is first opened, about 2 MB compressed).

How it works: the panel's browser client speaks RDP over a WebSocket to the panel; the panel carries the connection through the agent to the Remote Desktop service of the computer (`127.0.0.1:3389`, or the port set in Windows). Port 3389 is not opened to the network. You sign in with a Windows account (login, password, domain if any); the client signs in by itself (CredSSP), so the password is not stored and does not appear in the log. Only the login is remembered in the browser. Every open and close is written to the audit log with the login that was typed. The login and the password need not be typed: choose them from your personal encrypted file "Keys" (the list "Key" in the connection form), see [Keys](keys.md).

What you can do: mouse, keyboard, text clipboard in both directions (the browser asks for permission; over plain HTTP the browser gives no clipboard access, then the menu items are off; Ctrl+V first sends your clipboard to the computer, and after Ctrl+C or Cmd+C the clipboard of the computer comes into yours; **on a Mac Cmd works as Ctrl** (Cmd+C, Cmd+V, Cmd+A...), a Cmd tapped alone is the Windows key), the resolution follows the window ("More" > "Change resolution with the window"), scale "fit the window" or 100%, full screen. The bar of the window has **Ctrl+Alt+Del** and a "More" menu: Task Manager, Windows key, lock, Run, Alt+Tab, clipboard, scale, full screen, disconnect. Files go through the file manager.

Needs: Remote Desktop turned on in Windows (Pro, Enterprise and Server editions; not Home). If it is off, the window offers "Turn remote desktop on" (a confirmed action in the audit log: the setting, the firewall group "Remote Desktop" and the service). An account with a password that may sign in over Remote Desktop (administrators or the group "Remote Desktop Users"). One person at a time: a Windows client edition pushes the person at the console out; the panel opens at most one desktop per computer. For an operator the module "desktop" is given like the others (off by default).

Limits: no sound, no printers or disks (use the file manager), sign-in by NTLM (an account of a domain with "Restrict NTLM" will be refused), no Kerberos. The panel needs a CSP that allows WebAssembly (`wasm-unsafe-eval`); this is set by the server itself.

## Assistant and "Management"
The "Assistant" tab (next to "RDP", with its own "open in a window" button; the server card on the dashboard has an "Assistant" button too) shows the **screen of a Windows session** and lets you control it: the console, that is what the person at the computer sees, or the screen of another user's RDP session. No password is needed: access follows the Linager rights (the module "Assistant"), like Aspia or Radmin. There is no consent prompt for the user; every opening and closing is written to the audit log (the session, the duration, the picture settings).

Unlike "RDP" this is not a new RDP session: the person at the computer is not pushed out, and Windows Home works (it has no RDP). The session is chosen with the "Session" button on the window bar; the button with a monitor icon sets the display mode: in a floating window "Picture fits the window", "Stretch to the whole window" (the picture fills the window and takes its proportions; the resolution of the computer is not changed), "Window fits the picture" (the default: the window gets the size of the picture at 100% and follows it) and "Window fills the browser"; in the tab "Picture fits the window", "Stretch to the whole window" and "Picture at 100% (scrolls)"; "Full screen" is in both. a disconnected session has no screen and is greyed. One person at a time per session.

How it works: the agent (a service, SYSTEM) starts a helper in the session on demand (`linager.exe agent assistant-helper`, the same file) with the token of the service moved to the number of the session. The helper takes the screen with GDI (this works in RDP sessions too), compares the picture with the previous one in 64×64 tiles and sends the changed tiles as JPEG; the mouse and the keyboard come back and go to `SendInput` (keys as scan codes, the layout is Windows' own). The helper calls the agent back on `127.0.0.1` with a one-time token from an environment variable; the agent moves the bytes and reads only what goes up: it takes Ctrl+Alt+Del for itself (only a service can send it). The pointer of the computer is not in the picture by default: your own pointer is seen and moves without delay; a box in "Management" draws the pointer of the computer into the picture.

The **"Management"** tab (group "Network and protection", Windows servers only) sets the picture: the presets "Economy", "Normal", "Quality"; the colours (24 bit, 16 bit, 256 colours, shades of grey); the upper limit of the frame rate (1-30); the **traffic limit** in Mbit/s per session (or no limit). There is no real video codec, so the "bitrate" is a limit: the encoder lowers the JPEG quality and skips frames to keep to it (the box "Lower the quality by itself"). A change takes effect at once, open sessions included. **RDP** is turned on and off here too; while it is off (or Windows Home has none) the "RDP" tab and menu items on the server page and on the dashboard are hidden, and they come back at once when it is turned on, without reloading the page (the agent reports the state with every sample): turning it off (a confirmed action recorded in the audit log) forbids new RDP connections and closes the firewall rule, and does not break sessions that are open; the agent and the Assistant do not depend on RDP. Windows Home has no RDP and the button is unavailable. The table "Windows sessions now" shows who is signed in and from where and opens a session in the Assistant.

Rights: the module "Assistant" is given to an operator separately from "RDP" (off by default); turning RDP on and off needs the right "RDP". The module can be switched off on the computer itself in `C:\ProgramData\Linager\agent.yaml` (`assistant: false`).

**Lock screen, sign-in and UAC.** They run on a separate Windows desktop (Winlogon). Every second the helper checks which desktop Windows sends the input to and follows it, so the lock screen, the sign-in screen (when nobody is at the console) and a UAC prompt are seen and controlled; a badge "UAC" shows under the screen and in the header. On these screens the bar has a **key button** that types a login and a password from your personal file "Keys" (the login, the password or the whole sign-in), see [Keys](keys.md). **Ctrl+Alt+Del** works on the console (the service cannot send it to another RDP session, the button is off there); Windows has to allow it: "Management" has a switch for the policy "Software SAS generation" (a confirmed action, in the audit log). **Clipboard** (text up to 1 MB) works with the usual shortcuts: Ctrl+V sends the text of your clipboard and pastes it on the computer (the browser hands over the text by itself, no permission is asked), Ctrl+C and Ctrl+X bring what was copied on the computer into your clipboard (the browser may ask for permission; over plain HTTP it gives no access to the clipboard). On a Mac Cmd works as Ctrl (Cmd+C, Cmd+V, Cmd+A...), a Cmd tapped alone is the Windows key. The same is in the menu items "Send my clipboard" and "Get the computer's clipboard"; the text moves only when you ask. **Several monitors:** the "More" menu picks a monitor or "All monitors" as one picture. **Dispatcher:** the icon button on the bar (next to "Files", on every connection; whoever has the right "Processes" or "Services" sees it) opens a floating window in the spirit of the Task Manager, one per server, closed with the cross. The "Processes" tab: a list that refreshes every 3 s, sorted by any column (process, PID, user, CPU, RAM; a second click turns the order over), the totals of CPU and RAM in the headers, cells that get warmer with the load; at the bottom "End" and "Force end" (both ask for confirmation and are written to the audit log; the plain one ends that process only, the forced one also ends every process it started; Windows does not let a service close a window softly in the session of the person, so what is not saved inside the process is lost in both cases). System processes and the agent itself cannot be ended. The "Services" tab: the filters "All / Running / Stopped / Failed", sorting by a column, at the bottom "Restart" and "Stop" for a running service and "Start" for a stopped or failed one; protected services (the Linager agent, remote desktop and others) ask for confirmation. **The line under the screen** (resolution, colours, frames, traffic) is hidden by default and is turned on with the item "Information" of the "More" menu (a tick means shown); the choice is one for all windows and tabs of the Assistant and is remembered in the browser. There is no focus frame around the screen. The item "Quality and frame rate…" opens the same picture settings as the "Management" tab **in a small floating window** (one per server; it works in the Console too), so the quality can be changed beside the screen while the frames and the traffic are seen on the line under it.

Limits: no sound and no file transfer (use the file manager), no recording, no connecting to a disconnected session. The helper runs as SYSTEM, like the service: an antivirus or EDR may object to a process that takes the screen and types keys; `linager.exe` needs an exception.

## Files, log and removal
- `C:\ProgramData\Linager\agent\` keys and state; `C:\ProgramData\Linager\agent.yaml` (modules, optional, the same format as on Linux: `modules:` with `true` / `false`); `C:\ProgramData\Linager\agent.log` the log (a file over 5 MB is moved to `agent.log.1`). The folder `C:\ProgramData\Linager` is open to SYSTEM and the administrators only; this is the counterpart of the mode 600 of the Linux agent.
- `linager.exe agent status` shows the version, the server and the state of the service.
- After a change in `agent.yaml` restart the service: `Restart-Service LinagerAgent`.
- Remove: `msiexec /x linager-agent_windows_amd64.msi /qn`. The keys, the log and the address of the server stay; to remove them as well, delete `C:\ProgramData\Linager` and the key `HKLM\SOFTWARE\Linager`. "Delete server" in the panel makes the agent remove its identity and service.

## Updating
From the panel, like a Linux agent: the "Updates" page (one agent, selected ones or all). The server gives the agent the signed `linager-agent_windows_amd64.exe` of its release (the same file the MSI installs). The agent checks the signature of the manifest and the checksum, saves the running exe as `C:\ProgramData\Linager\agent\linager.prev.exe`, moves the running file aside (Windows allows renaming a running program, not overwriting it), puts the new one in its place and restarts the service. The stages are the same as on Linux (download, verify, backup, replace, restart, offline, reconnect, confirm).

**Rollback.** Before the restart the agent creates a one-shot task of the Windows Task Scheduler, `LinagerAgentRollback` (runs as SYSTEM, about 70 seconds later). The new version confirms itself with its first connection to the server and deletes the task. If it does not (it cannot start, or cannot reach the server), the task runs the OLD exe: it stops the service, puts the old file back and starts the service again. By hand: `linager.exe agent update`, `linager.exe agent status`.

**An agent from a pre-release build older than `0.3.0-beta.4` cannot update from the panel** (it does not have the code for it): the table says "new MSI X". Install the MSI of `0.3.0` or newer once; after that the panel updates the agent. The same table message appears when a release has no Windows file. The program list of Windows (the MSI product) keeps showing the version of the MSI after an update from the panel; this changes nothing in how the agent works, and the next MSI installs over it as usual. The server of the release must be updated first, as with Linux agents.

## Limits
- Only x64. The agent is updated from the panel; the rollback of a failed update was tested on a Windows runner and on one test machine only. The exe and the MSI are not signed with a code signing certificate, so SmartScreen or an antivirus may warn about them.
- Checked: the code is built and tested on a Windows runner of GitHub Actions (metrics, services, processes, files, terminal, install and removal of the MSI). Not checked on a real fleet.
- Windows reports one product name "Windows 10" for Windows 11; the agent tells them apart by the build number (22000 and up).
