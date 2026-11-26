# Devices

Settings in the application → **Devices**: the list of the applications (the desktop application, the iPhone application, Android later) that sign in to the server, and the confirmation of new devices. The section is for an administrator only.

## What a device is

At its first start the application makes a random secret token (256 bits) and keeps it in a protected store: the Keychain on an iPhone, the key store of the system on a computer. The token goes to the server in the header `X-Linager-Device`; the server keeps only its hash. The header `X-Linager-Client` tells what the application is (`platform=ios&app=0.1.0&os=27.1&model=iPhone18%2C1`). A device belongs to one user: one phone under two accounts is two devices. Installing the application again makes a new device.

The list shows the device (the model and your own name for it), the user, the versions of the application and of the OS, the last IP, the time of the last activity and the state. A device takes one compact row and the buttons are always in view: a device that waits has "Approve" and "Deny" (its row is highlighted and stands first), every device has "Name" and "Remove". The server takes the last IP from the requests themselves, not from what the application says. The list is kept always, with the confirmation off too.

## The confirmation

After the server is installed the confirmation is **off**. An administrator turns it on in the application (the switch "Confirm devices"). From then on **every** connection, even from an allowed IP, needs a confirmation, as with agents installed by hand:

1. A new device signs in with the login, the password and the 2FA code. The session opens, but the device is "waiting for approval": the server answers everything but "who am I" and the sign-out with 403 `approval_pending`. The device shows a "Waiting for approval" screen.
2. An administrator sees the request in "Devices" (the device, the user, the version, the IP, the time) and presses "Approve" or "Deny". An administrator may approve themselves, but only from the desktop application of a device that is approved already.
3. An approved device passes without questions at its next sign-in. A denied one cannot sign in (403 `device_denied`). "Remove" erases the device and signs it out at once.
4. Every decision goes to the audit log: `device.request`, `device.approve`, `device.deny`, `device.remove`, `device.label`, `device.gate.on`, `device.gate.off`.

While the confirmation is on, the cookie of the session is not enough: every request must come with the token of the device. A stolen cookie without the token does not work (401). A client with no token at all (an old version of the application) cannot sign in (403 `device_required`): update the application.

### Turning it on

It can be turned on only from an application that has a device already: that device is approved by itself, or the administrator would lock themselves out. The server asks for a confirmation and offers a choice:

- **Keep the open sessions**: the devices that are signed in stay, and are taken as approved (look through the list and remove what is not yours);
- **Sign out everywhere but this device**: all the others sign in again and go through the confirmation. Choose it when you doubt the open sessions.

### If the way in is lost

If the confirmation is on and no approved device is left, run on the server:

```bash
linager server devices list          # the devices and the state of the confirmation
linager server devices approve <id>  # approve a device
linager server devices off           # turn the confirmation off (no restart)
```

The commands write to the audit log (the actor is `cli`). Whoever can run them owns the server already.

## What it does not do

The confirmation does not replace the password, the 2FA and the restriction by address; it is a third barrier on top of them: somebody else's phone with a leaked password and code does not get in without your knowledge. The sessions that are open when it is turned on (the choice "keep the open sessions") are taken as approved.
