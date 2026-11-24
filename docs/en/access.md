# Admin panel access

Settings → **Admin access** restricts who may open the admin panel and shows a journal of visits. The restriction applies to the admin panel port only; the agent port is not affected, so servers stay connected.

![Access restriction](../screenshots/en/access-restriction.png)

## Modes

| Mode | Behavior |
|---|---|
| Off | The panel is open to everybody who reaches the port. |
| White list | Whatever is not listed is refused. Good when you know where you sign in from. |
| Black list | Whatever is not listed is allowed. Closes only the addresses you know are unwanted. |

Each mode has its own list of addresses, networks (`203.0.113.7` or `203.0.113.0/24`, IPv6 too) and host names (see "Host names and DDNS") with a comment. The button "+ Private networks (RFC 1918)" adds `10.0.0.0/8`, `172.16.0.0/12` and `192.168.0.0/16` to the white list. "Check an address" tells what the server would do with an address, through a trusted proxy or directly.

## Host names and DDNS

If you have no fixed public address but you have a DDNS name (for example `home.example.duckdns.org`), put the name into the list instead of an address. It carries the tag `DDNS`, and the column "Points to now" shows the addresses the name resolved to (IPv4 and IPv6) and when it was last checked.

- The server reads the names again in the background once a minute and puts new addresses in force at once. The button "Refresh" does it immediately. A name is added only when DNS knows it.
- When DNS stops answering, the last known address stays in force (it is kept in the database and survives a restart of the server). If there has been no answer for more than a day, the address stops counting: a name that has been gone for long must not keep an address open for ever. So after a long DNS outage a white list may lock you out as well; `linager access reset` helps then.
- Names are allowed in the white and black lists only. They cannot be trusted proxies: DNS would let `X-Forwarded-For` be forged. A name needs a dot and the Latin form (an IDN is written as `xn--...`); a list holds at most 20 names.

::: warning What to know about DDNS
- **A delay after the address changes.** From the moment the provider changes your address until the DDNS record and the server catch up (up to a minute plus the TTL of the record), the old address stays allowed. Another subscriber of the provider may get it.
- **CGNAT.** If the provider shares one public address between many subscribers, a white list by such an address protects little: any of them can get in.
- **Trust in DNS.** Whoever can change the DDNS record (for example by stealing the password of the service account) gets into the panel. Turn on two-factor sign-in for the DDNS account.

The most reliable way is to come in through a VPN (WireGuard, Tailscale) and let only its address into the panel.
:::

## Trusted proxies

Behind a reverse proxy such as Nginx Proxy Manager the connection arrives from the proxy, not from the visitor. The server reads the real address from `X-Forwarded-For` **only** when the connection comes from a trusted proxy. A connection from any other address is judged by its own address and the header is ignored, so the header cannot be forged and the local network can connect directly.

When you open the section through the proxy, it shows how the request arrived and offers "Add … as a trusted proxy". Add the proxy and remove the default `0.0.0.0/0` (everybody trusted): while everybody is trusted, the lists cannot be turned on.

::: warning Do not leave the proxy untrusted
If the proxy is not trusted and the white list holds your whole local network, every visitor looks like the proxy (a local address) and the internet gets in. The panel warns about it and refuses to save such settings.
:::

The switch **Only through trusted proxies** refuses direct connections to the port (a local network included), even when the address is in the list.

If Nginx Proxy Manager sits behind Cloudflare, turn on passing the real visitor address in NPM, otherwise you will see the address of Cloudflare. If NPM runs in Docker on the same host, Linager may see it from the gateway of the Docker network (for example `172.17.0.1`): add exactly the address the interface shows.

## What cannot be saved

The panel checks every change and refuses to apply it when:

- it would lock you out (your own address would be refused);
- everybody is trusted (`0.0.0.0/0`);
- a proxy sends `X-Forwarded-For` but is not in the trusted list;
- the white list is empty;
- a host name of a list does not resolve to an address (DNS does not know it).

Applying asks for a confirmation and is written to the audit log (`access.update`).

## Access journal

![Trusted proxies and the access journal](../screenshots/en/access-journal.png)

The journal records page opens, sign-ins (good and failed) and refused requests with the client address and the proxy they came through. Filters: all, refused, sign-ins, failed sign-ins, through a proxy, direct. "To white list" / "to black list" puts an address into the list in one click. Retention is 7, 30 or 90 days. Background requests of the interface are not written.

## Server update and the restriction

After an update the server checks that the new version came up: the updater asks the panel root from the server itself (directly, without `X-Forwarded-For` and not as a browser). This request is let through the restriction, so 127.0.0.1 does not have to be in a list. Every other address and path, and a browser on the server itself, are judged by the rules as usual.

## If you lock yourself out

On the server:

```bash
sudo linager access reset
```

The restriction turns off (the admin panel and the application port), the lists stay. A running server applies it within a few seconds.

## Server access (the application port)

The application port has its own lists: Settings in the application → **Server access**. It is a white or black list of addresses and networks (addresses, networks and host names like in the admin panel: the addresses of a name come from DNS once a minute), a test of an address and the journal of the connections of the application. The port is direct, without a proxy: the address of the connection itself is judged and `X-Forwarded-For` is dropped. Until the list of the port is saved it follows the restriction of the admin panel (as before); after the first "Apply" they are independent. A list that would close the port to you yourself cannot be applied. `linager access reset` turns both restrictions off.
