# The Linager application

The desktop application (macOS, Windows, Linux) shows the interface of a server in native windows. It will replace the web panel: `web/` is frozen and will be removed after the transition. The server is installed with the terminal installer (`linager install server`); the application connects to a ready server.

**State (stage 1, beta).** Done: the connection with a certificate check, sign-in (login, password, 2FA), the dashboard in three layouts (cards, compact cards, rows), light and dark themes, Russian and English, a terminal in a window of its own. Not yet: the server page, files, the editor, the Console, RDP and the Assistant, settings, the tray and notifications, self-update, sign-in with a passkey.

## Connecting

1. The **application port** is on in the server's "Settings → Connection" (the installer asks for it). The certificate fingerprint is shown there too.
2. Enter `address:port` in the application. It connects and shows the fingerprint of the server certificate.
3. Compare it with the fingerprint in the server settings or in the installation summary and confirm. The application remembers it and accepts only this certificate from then on (the way SSH remembers a host key). If the certificate ever changes, the application refuses to connect and warns you.
4. Sign in. The password can be saved in the keychain of the system (Keychain on macOS, Credential Manager on Windows, Secret Service on Linux); then sign-in is automatic, and the 2FA code, if it is on, is still asked.

The list of servers and the last login are in `<user config dir>/Linager/profiles.json` (mode 600, nothing secret: a fingerprint is public).

## How it works

Inside the application a local server listens on `127.0.0.1`. Every path on it starts with a random token (no other program or web page knows it), and a request with another `Host` is refused (protection against DNS rebinding). The windows open its address.

- The interface is served by the application itself.
- The API of the server is proxied over TLS 1.3 with the fingerprint pinned. It is checked on every connection. The session cookie is kept by the application and never reaches the page. SSE (live data) and WebSocket (the terminal) go through the same proxy.
- Its own calls (`/app/...`): profiles, the keychain, opening windows.

The interface (`app/frontend`, Svelte) does not depend on Wails, so it can also be opened in a browser: `make -C app headless` prints the address.

## Getting the application

There are no ready files in the releases yet, the application is built from the sources (Go, Node.js and the Xcode Command Line Tools are needed):

```bash
make -C app bundle   # app/bin/Linager.app
make -C app zip      # app/bin/Linager-macos-arm64.zip (x86_64 on Intel), to hand it to another Mac
```

The build is signed ad hoc only, not by a developer, and not notarized. Open it from the context menu the first time (right click, "Open"). If macOS says the file is damaged (it does with a zip downloaded from the network), remove the quarantine: `xattr -dr com.apple.quarantine Linager.app`.

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
