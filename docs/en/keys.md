# Keys

"Keys" is your personal encrypted file of logins and passwords. It saves typing them: in the RDP form you choose a key and the login, the domain and the password are filled in. The item "Keys" is in the menu of the top bar of every user of the panel (an administrator, an operator, an auditor); everyone has their own file. While the vault is open, the item in the menu has an open lock icon of the dark orange colour.

## The main points

- **The file is kept by you, not by the server.** The database of the panel, `/etc/linager` and the audit journal have neither the keys nor the passphrase. A leaked database or a compromised panel server does not reveal the keys.
- **Decrypted in the memory of the tab only.** The file is opened once per tab (choose the file and enter the passphrase); the decrypted data lives in the memory of the page and is saved nowhere: not in `localStorage`, not in `sessionStorage`, not in IndexedDB. Closing the tab, reloading the page, signing out and the auto-lock forget everything.
- **The password goes only to the RDP client** (in the application). It is not sent to the panel server.
- **The passphrase cannot be recovered.** A forgotten passphrase means a lost file. Make a copy of the file and keep it apart.

## How to use it

1. Menu of the top bar → **Keys**.
2. **Create a vault:** a passphrase (at least 10 characters, several words are better) twice, "Create and download the file". The browser downloads `linager-keys.lvault`; keep it in a safe place.
3. **Open a file:** "Choose", the `.lvault` file, the passphrase, "Open". Decrypting takes from one to a few seconds, on purpose.
4. The window then shows the table of the keys: **Add a key**, **Edit**, **Delete** (confirmed in the row), a search and a filter by type.
5. **Saving is automatic.** Every change (add, change, delete a key) is written into the open file half a second later, with no dialog and no button: through a temporary file, so a break cannot leave half a file; the previous content stays beside it as `<file>.bak`. If the file on the disk changed meanwhile (a sync program, another computer) the application asks: overwrite, or save under another name. If it cannot write (the file was deleted, no rights) the window says "could not save" and shows the button "Save the file". The file is still created through a dialog. A browser (development) has no autosave and keeps the button. The encryption is the same: only encrypted bytes go into the file.
6. **Close the window** leaves the vault open until the tab is closed. **Close the vault** forgets the decrypted data at once (with unsaved changes it asks what to do).

The auto-lock (15 minutes of silence by default; 5, 30 or off; the choice is remembered in the application) closes the vault by itself. With unsaved changes it stays open until you save the file, and closes then.

The dot on the key button (in the top bar and on the Assistant and RDP panel) shows the state: green, the vault is open; amber, the auto-lock time came but there are unsaved changes (save the file); red, the vault is closed. The red dot shows only when a vault is known (a file was opened in this window or a file is remembered); otherwise there is no dot. Each application window shows its own state.

## A key

| Field | What it is |
|---|---|
| Name | Shown in the lists instead of the login and the password. |
| Use for | **RDP**, **UAC** or **Any**. The RDP form offers the keys RDP and Any. The type UAC is for typing into the UAC prompt and the Windows sign-in screen in the Assistant (see below). |
| Binding | To folders and servers of the panel (several of them): the servers are laid out in folders, the switch of a folder covers all its servers and the folders under it, the ones that appear later too. Nothing chosen: the key is for all servers. |
| Login, domain | The domain is optional. |
| Password | Can be shown, and a random one (20 characters) can be generated. |
| Note | Optional. |

**A binding only sets the order in the lists, it is not protection.** In the RDP form the keys bound to this server come first, then those of its group, then those for all; the rest are behind "Show all". Every key of the file is open to whoever opened the file. If the servers or groups a key is bound to are removed from the application, the key gets the badge "not found", and a key with no living binding counts as general; the dead binding can be taken off in the window of the key.

## The RDP form

In the connection form (the tab "RDP" and the floating window) there is a list **"Key"** above the login. When the vault is closed, the list has the button "Open": it opens a small window with the file and the passphrase (not the whole window "Keys"), and once the vault is open the list of keys unfolds by itself. Choosing a key **connects at once**: the login, the domain and the password go to the RDP client in the application (CredSSP), the password is not sent to the panel server. If the connection fails, the form comes back with the fields filled. The list opens over the page like the other drop-down lists (as a sheet from the bottom on a phone) and does not make the window taller. The connection is written to the audit with the login, as before.

## Typing on the lock screen, the sign-in screen and into a UAC prompt

In the Assistant (the tab and the floating window) of a Windows computer, while the screen is the secure desktop (the lock screen, the sign-in or a UAC prompt; the header has the badge "UAC"), the bar has a **key button**. It opens the list of the keys of the type **UAC** and **Any** (of this server, of its group, for all; the rest behind "Show all"). Every key has three buttons:

- **Login** types the login (with the domain: `DOMAIN\login`) and presses Tab: the focus goes to the password field;
- **Password** types the password and presses Enter: for when the login is already on the sign-in screen;
- **Sign in** types the login, presses Tab, types the password and presses Enter.

When the vault is closed the list has the button "Open" (a small window with the file and the passphrase), and the list unfolds after it.

How it works and what to expect:

- The text is typed as **characters** (`SendInput` with `KEYEVENTF_UNICODE`), not as keys: the keyboard layout of the computer does not matter and the clipboard is not used, so the password does not stay in it.
- **The agent types only on the secure desktop and checks it at the moment of typing.** If a UAC prompt went away between the click and the typing, the password is not typed into the window of the person: the helper refuses and the panel shows "Typing refused".
- **The password passes through the panel server in memory**: the browser, the server, the agent (through the same protected channels as the screen). The server writes it neither to the database nor to the journal, but it sees it in memory while it is typed. Unlike RDP, where the password stays in the application, this cannot be avoided here: the agent types it. There is no "browser to agent" encryption on top of the channel yet.
- The audit journal gets `assistant.type`: the session, the **name of the key** (the browser sends it), the number of characters and what is pressed after (Tab, Enter); **no text**. `assistant.close` has the number of typings (`typed`). At most 60 typings per opening of the Assistant.
- **After the sign-in the Assistant connects again by itself.** Signing in changes the session of the computer and the old connection ends. If the connection ends within 30 seconds after the screen was the sign-in (or after a key was typed), the panel connects again silently (at most 12 tries in a minute, two seconds apart: the desktop does not appear at once); the picture and the size of the window stay until a new frame comes. This works for the session "console"; if a session number is picked, the connection goes to the same number.
- Rights: the same as for the Assistant (the module "Assistant"); there is no separate right.
- Windows only. A Linux computer (GNOME, the sign-in screen) has no button. Update the agents to this version: an old agent shows the button but answers that typing is not supported.
- If the sign-in screen offers a PIN or Windows Hello by default, open "Sign-in options" first and choose the password: Windows decides where the text goes.

## The file and the encryption

The format `.lvault`: a header (the mark `LGRK`, the version, the parameters, a salt, a nonce) and the encrypted content. The encryption key comes from the passphrase with **Argon2id** (64 MiB of memory, 3 passes, 1 lane; the parameters are written in the file), the content is encrypted with **AES-256-GCM**, and the header is authenticated: a changed byte anywhere in the file gives an error, not garbled data. The passphrase is brought to one Unicode form (NFKC), so the same word typed on another keyboard opens the file. The file is at most 1 MB and has at most 500 keys; a file with foreign parameters that ask for too much memory is not opened. The cryptography (`@noble/hashes`, `@noble/ciphers`) is plain JavaScript: it works over HTTP, where `crypto.subtle` is not available, and needs no WebAssembly.

## The iOS application

The same file opens on an iPhone too ([ios.md](ios.md)): read only, and signing in with a key in the Assistant; the keys are made and edited in the desktop application.

## What is not included

"Browser to agent" encryption of the password when it is typed into UAC, typing on the usual desktop, syncing the file between devices (the file can be put in iCloud, Syncthing and the like; two copies edited at once are not merged), a shared team file with access control, history and attachments.
