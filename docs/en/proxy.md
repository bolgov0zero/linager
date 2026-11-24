# Reverse proxy

The Linager web panel is gone (removed in 0.10.0-beta.17), so a reverse proxy (Caddy, nginx, Nginx Proxy Manager) is neither needed nor supported for the server. The interface of the server is the Linager application, which connects to the application port directly over TLS with a pinned certificate: see [connection](connection.md) and [the application](app.md). Who may connect is set in Settings → Server access: [access.md](access.md).

If a Caddy or nginx that an earlier version of the installer set up for the panel is still there, it can be removed: Linager no longer relies on it.
