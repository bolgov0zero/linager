# Server access

Settings in the application → **Server access** restricts who may connect to the application port and shows a journal of the connections. This is the only way to the interface (there is no web panel any more), so the restriction applies to all the management. The agent port is not affected, the servers stay connected.

The application port accepts a direct TLS connection, nothing stands in front of it. The address of the connection itself is judged; `X-Forwarded-For` and the like are dropped.

## Modes

| Mode | Behavior |
|---|---|
| Off | Anyone who reaches the port may connect. |
| White list | Whatever is not listed is refused. Good when you know where you connect from. |
| Black list | Whatever is not listed is allowed. Closes only the addresses you know are unwanted. |

Each mode has its own list of addresses, networks (`203.0.113.7` or `203.0.113.0/24`, IPv6 too) and host names (see "Host names and DDNS") with a comment. The button "+ Private networks (RFC 1918)" adds `10.0.0.0/8`, `172.16.0.0/12` and `192.168.0.0/16` to the white list. "Test an address" shows what the server would do with the given address.

## Host names and DDNS

If you have no permanent external address but have a DDNS name (for example `home.example.duckdns.org`), put the name in the list instead of an address. It is marked `DDNS`, and the column "Points to now" shows the addresses it resolved to (IPv4 and IPv6) and the time of the last check.

- The server looks the names up in the background once a minute and puts new addresses in force at once. The button "Refresh" does it immediately. A name is added only when DNS knows it. The server resolves the name, not the application: it has its own DNS and it is the one that applies the list. When the common lookup of the IPv4 and IPv6 addresses fails (it happens with the DNS proxies of home routers), the server asks for each family one after the other; on an error the message and the row of the table show what DNS answered.
- If DNS stops answering, the last known address stays in force (it is kept in the database and survives a restart of the server). If there has been no answer for more than a day, the address stops counting: a name that has been gone for long must not keep an address open for ever. Because of this a long DNS outage may lock you out too: `linager access reset` helps then.
- Names with a dot in Latin spelling are accepted (Cyrillic domains are written as `xn--...`); a list holds up to 20 names.

::: warning What to know about DDNS
- **Delay after an address change.** While the provider has changed the address and the DDNS record and the server have not caught up (up to a minute plus the TTL of the record), the old address stays allowed. Another subscriber of the provider may get it.
- **CGNAT.** If the provider shares one external address between many subscribers, a white list by such an address protects little: any of them can connect.
- **Trust in DNS.** Whoever can change the DDNS record (for example by stealing the password of the service account) gets access to the server. Turn on two-factor sign-in in the DDNS account.

The most reliable way is to connect through a VPN (WireGuard, Tailscale) and let only its address reach the port.
:::

## What cannot be saved

The server checks every change and does not apply it if:

- it would lock you out (your address would be refused);
- the white list is empty;
- a host name of the list does not resolve to an address (DNS does not know it).

Applying needs a confirmation and is written to the audit log (`appaccess.update`).

## The journal of connections

The journal holds the sign-ins (successful or not) and the refused requests with the address of the client. Filters: all, refused, sign-ins, failed sign-ins. "To white list" and "to black list" add an address with one click. Entries are kept for 30 days. Background requests of the application are not written.

## If you lock yourself out

On the server:

```bash
sudo linager access reset
```

The restriction of the application port turns off, the lists stay. A running server applies it within a few seconds.

## Carried over from the admin panel

Before 0.10.0-beta.17 the admin panel (the web panel) had a restriction of its own, and the application port followed it until the port had lists of its own. At the first start of the new version the server carries those lists (the mode, the white and black lists, host names included) to the lists of the application port once, so that a port that was closed stays closed. The trusted proxies and "only through a trusted proxy" are not carried: they no longer exist.
