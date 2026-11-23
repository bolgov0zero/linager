# Connection: agent and application ports

"Settings" → **Connection** shows and changes two ports of the server. A change takes effect at once, the server does not need a restart. The section is visible to the administrator.

| Port | What is on it | Who connects |
|---|---|---|
| Agent port | TLS 1.3, mutual certificate check | agents on the managed servers |
| Application port | TLS 1.3 with the certificate of the agent port, the same API as the admin panel | the Linager application |

The admin panel port (behind Caddy or your own proxy) is not changed here: it is set at installation in `server.yaml`.

Both ports use one certificate, so the section shows one fingerprint (SHA-256). The application remembers it on the first connection, the way SSH remembers a host key.

## Application port

A separate port: a direct TLS connection without a reverse proxy. The installation wizard always asks for it (`--app-port` without questions). On a server installed earlier the port is not set: enter it in "Connection". An empty field switches the port off.

- Reverse proxy headers (`X-Forwarded-For`, `X-Real-IP`, `Forwarded` and the like) are dropped on this port: a client cannot name its own address. The admin access restriction and the sign-in limits see the real address of the connection.
- The admin access restriction ("Admin access") applies to this port too: the white and black lists judge the real address of the connection. The rule "only through a proxy" does not apply to it: the port is direct by design.
- Open the port in the firewall (the wizard reminds you when nftables is active).
- The value set in "Connection" is kept in the database and wins over `server.yaml` (the service cannot write to `/etc/linager`). The wizard writes it to `server.yaml`: `app.listen: "0.0.0.0:PORT"`.

## Changing the agent port

An agent stores the server address, so the port cannot simply be switched: the agents would lose the connection. The move works like this:

1. The server opens the new port and **keeps the old one open**.
2. It sends every connected agent a signed message with the new port. The agent keeps its host, only the port changes.
3. The agent saves the new address and tries it first. The first connection that works makes the new port the main one.
4. If the new port does not work (a firewall, a typo), the agent goes back to the old port after three failed attempts and ignores that port for an hour.
5. An agent that was offline gets the message when it connects to the old port.
6. When every registered agent has moved, the old port closes by itself. The section shows the progress: "N of M moved" and the list of those still waiting.

Limits:
- An agent follows the move from version **0.10.0-beta.1**. While an older agent is online the agent port cannot be changed: the section lists such agents. Update them, then change the port. An old agent that is offline does not block: it waits on the old port until you update it.
- While a move is in progress the agent port cannot be changed again.
- **"Close the old port"** closes it at once; agents that did not move lose their connection (with a confirmation).
- Install links and commands made earlier contain the old port and stop working when it is closed.
- Open the new port in the firewall of the server and the network beforehand.

Every change is written to the audit log (`connection.update`, `connection.close_old`).

## API

`GET /api/connection`, `PUT /api/connection` (`{"agent_port": N, "app_port": N}`, 0 switches the application port off), `POST /api/connection/close-old`. Administrator only.
