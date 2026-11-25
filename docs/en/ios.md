# The iOS application

A separate native iPhone application (SwiftUI), not a wrapper around the web. The first stage is **the dashboard only**, read-only. Editing the dashboard, folders and tags, the console, the terminal, files, users and the other modules are not in it. The source is in `ios/`, the mockup of the screens in `docs/design/ios/index.html`.

## What it does

- Connects to a server by address and application port. The certificate is pinned on the first connection (the way SSH remembers a host key): the application shows the fingerprint, the person compares it with the server and confirms.
- Signs in with a login and a password, then a TOTP code or a backup code as a second step.
- The dashboard: four summary tiles (online, attention, updates, CPU), the segment "All / Problems / Offline", a search by name or address, folders as sections (a subfolder is named by its path), two layouts (rows, and cards with a CPU chart), an order and a folder filter, pull to refresh.
- State badges are the compact ones only, as in the desktop application: a symbol and a number (failed services, reboot, updates, Docker images, agent), a word only for "No connection". Tags are letter squares; the full names and the meaning of the badges are in the long-press menu of a row (which also copies the name and the address).
- Live data: the stream `api/live` refreshes the dashboard and is opened again when it breaks. In the background the stream is closed; on return the data is asked for again.
- Without a connection to the server the last received data is shown with its time.
- A Face ID lock (optional): the screen is covered while the application is in the background.
- Russian and English, light and dark theme, Dynamic Type and VoiceOver.

## Requirements

An iPhone with iOS 27, portrait. A server with the application port (0.10.0-beta.1 or newer). The server has to return `fingerprint_alt` (see below), otherwise the fingerprint cannot be compared by eye.

## First connection and the fingerprint

The server has two certificates with one name (Ed25519 and ECDSA, [connection.md](connection.md)). Apple's TLS gets the ECDSA one, so the fingerprint on the iPhone **does not match** the line `Certificate fingerprint`. Compare it with the line `Fingerprint for iPhone and iPad` of the command:

```bash
sudo linager server app-port
```

If the server prints one line only it is older than this change: update it.

If the fingerprint changes later (the certificate was renewed, or the connection is intercepted) the application does not go on and offers to compare the two fingerprints; the new one is pinned only after a confirmation.

## Build and install

You need Xcode 27 and XcodeGen (`brew install xcodegen`). No signing and no Apple ID is needed to build:

```bash
make ios-ipa
```

The file `ios/build/Linager.ipa` is built **without a signature**. Your tool (Sideloadly, AltStore, SideStore) signs and installs it with your Apple ID. Limits of a free Apple ID: the signature lasts 7 days (install again after that), up to three applications. Nothing is published to the App Store or to GitHub.

## Structure

| Part | What is in it |
|---|---|
| `ios/Sources/LinagerKit` | Everything but the screens: the certificate pinning (`URLSession` checking the SHA-256 of the leaf certificate before any data is sent), the API client, the event stream, the state of the application, the logic of `status.ts` (levels, thresholds, badges, totals), folders |
| `ios/App` | The SwiftUI screens, the strings (`Localizable.xcstrings`), the icons |
| `ios/project.yml` | The project for XcodeGen (the `.xcodeproj` is not kept) |

App Transport Security is switched off (`NSAllowsArbitraryLoads` in `project.yml`): with it an iPhone broke the TLS connection to the self-signed certificate of the server (error `-1200`, `-9802`) after the application had accepted the fingerprint. The application talks only to the server you name and trusts one certificate there, the pinned one, so the checks of ATS added nothing. Checked on a live phone.

The profile (address, fingerprint, session) lives in the Keychain on this phone only (`WhenUnlockedThisDeviceOnly`). The last dashboard is cached in Caches under file protection and erased on sign-out. The session cookie is kept by hand, not in a shared store.

## Tests

```bash
make ios-test
```

These are the tests of `LinagerKit` on the Mac, without a simulator: parsing of the server answers, thresholds and badges, the folder tree and filters, address parsing, the state of the application on a fake server and **a real TLS connection** to a local server (the fingerprint, a foreign certificate, the sign-in with TOTP, the cookie, CSRF, the event stream). The screens are not covered by tests: a person checks them on the phone.
