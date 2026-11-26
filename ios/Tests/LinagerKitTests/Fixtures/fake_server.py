#!/usr/bin/env python3
"""A tiny stand-in for the application port of the Linager server, for the tests of LinagerKit.
usage: fake_server.py CERT KEY DASHBOARD.json GROUPS.json  (prints the port, then serves until killed)"""
import base64, hashlib, json, os, ssl, struct, sys, time
from urllib.parse import urlparse, parse_qs
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

cert, key, dash, groups = sys.argv[1:5]
DASH = open(dash, 'rb').read()
GROUPS = open(groups, 'rb').read()
TOKEN, CSRF = 'tok123', 'csrf-abc'
RAN = []
SEEN = []  # (device token, client description) of every request that came with a session
APPROVED = [False]

def device_body(dev):
    # the device of the request as the server of the application tells it: tokens that start with P wait until approved, D are refused
    if not dev: return None
    if dev.startswith('P'): return {'confirm': True, 'id': 7, 'status': 'approved' if APPROVED[0] else 'pending'}
    if dev.startswith('D'): return {'confirm': True, 'id': 8, 'status': 'denied'}
    return {'confirm': False, 'id': 9, 'status': 'approved'}
SETTINGS = {'colors': '24', 'fps': 15, 'mbit': 4, 'nolimit': False, 'auto': True}
TILE = open(os.path.join(os.path.dirname(os.path.abspath(__file__)), 'tile.jpg'), 'rb').read()

def ws_frame(opcode, payload):
    n = len(payload)
    head = bytes([0x80 | opcode])
    if n < 126: head += bytes([n])
    elif n < 65536: head += bytes([126]) + struct.pack('>H', n)
    else: head += bytes([127]) + struct.pack('>Q', n)
    return head + payload

def read_exact(f, n):
    b = b''
    while len(b) < n:
        c = f.read(n - len(b))
        if not c: raise EOFError
        b += c
    return b

def ws_read(f):
    h = read_exact(f, 2)
    op, ln = h[0] & 0x0F, h[1] & 0x7F
    if ln == 126: ln = struct.unpack('>H', read_exact(f, 2))[0]
    elif ln == 127: ln = struct.unpack('>Q', read_exact(f, 8))[0]
    mask = read_exact(f, 4) if h[1] & 0x80 else None
    data = read_exact(f, ln)
    if mask: data = bytes(b ^ mask[i % 4] for i, b in enumerate(data))
    return op, data

class H(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    def log_message(self, *a): pass
    def send(self, code, body=b'', ctype='application/json', extra=()):
        self.send_response(code)
        self.send_header('Content-Type', ctype)
        self.send_header('Content-Length', str(len(body)))
        for k, v in extra: self.send_header(k, v)
        self.end_headers()
        self.wfile.write(body)
    def err(self, code, msg): self.send(code, json.dumps({'error': msg}).encode())
    def authed(self): return ('linager_session=' + TOKEN) in self.headers.get('Cookie', '')
    def do_GET(self):
        p = self.path
        if p == '/api/test/ops': return self.send(200, json.dumps(OPS).encode())
        if p == '/api/test/seen': return self.send(200, json.dumps(SEEN).encode())
        if p == '/deny': return self.send(403, b'forbidden\n', 'text/plain')
        if not self.authed(): return self.err(401, 'unauthorized')
        dev = self.headers.get('X-Linager-Device', '')
        SEEN.append([dev, self.headers.get('X-Linager-Client', '')])
        if dev.startswith('D'): return self.err(403, 'device_denied')
        if dev.startswith('P') and not APPROVED[0] and p != '/api/me': return self.err(403, 'approval_pending')
        if p.startswith('/api/servers/') and p.split('?')[0].endswith('/stream'): return self.terminal(p)
        if p.startswith('/api/servers/') and p.split('?')[0].endswith('/assistant'): return self.assistant(p)
        if p.startswith('/api/servers/') and p.split('?')[0].endswith('/desktop'): return self.desktop(p)
        if p.startswith('/api/servers/'): 
            if self.server_page(p): return
        if p.startswith('/api/servers/') and p.endswith('/assistant/settings'): return self.send(200, json.dumps(SETTINGS).encode())
        if p == '/api/snippets': return self.send(200, json.dumps({'snippets': [{'id': 1, 'name': 'Disk usage', 'command': 'df -h', 'position': 0}, {'id': 2, 'name': 'Two lines', 'command': 'cd /var/log\nls', 'position': 1}]}).encode())
        if p.startswith('/api/servers/') and p.endswith('/users'):
            if p.split('/')[3] == 'denied': return self.err(403, 'forbidden')
            return self.send(200, json.dumps({'users': [{'name': 'root', 'human': False}, {'name': 'debian', 'human': True}, {'name': 'www-data', 'human': False}, {'name': 'anna', 'human': True}], 'groups': []}).encode())
        if p == '/api/me':
            me = {'csrf': CSRF, 'login': 'admin', 'role': 'admin', 'totp_enabled': False}
            if device_body(dev) is not None: me['device'] = device_body(dev)
            return self.send(200, json.dumps(me).encode())
        if p == '/api/dashboard': return self.send(200, DASH)
        if p == '/api/groups': return self.send(200, GROUPS)
        if p == '/api/updates': return self.send(200, json.dumps({'agents': [{'id': 'srv-b', 'status': 'available'}, {'id': 'srv-a', 'status': 'current'}]}).encode())
        if p == '/api/live':
            self.send_response(200)
            self.send_header('Content-Type', 'text/event-stream')
            self.send_header('Cache-Control', 'no-store')
            self.send_header('Connection', 'close')
            self.end_headers()
            self.wfile.write(b'retry: 3000\n\n'); self.wfile.flush()
            for ev in ('metrics', 'status', 'dashboard'):
                time.sleep(0.05)
                self.wfile.write(('event: %s\ndata: {}\n\n' % ev).encode()); self.wfile.flush()
            time.sleep(0.05)
            self.close_connection = True
            return
        self.err(404, 'not found')
    def terminal(self, p):
        u = urlparse(p)
        sid = u.path.split('/')[3]
        q = parse_qs(u.query)
        if sid == 'off': return self.err(503, 'agent is offline')
        if sid == 'denied': return self.err(403, 'forbidden')
        key = self.headers.get('Sec-WebSocket-Key')
        accept = base64.b64encode(hashlib.sha1((key + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').encode()).digest()).decode()
        self.send_response(101)
        self.send_header('Upgrade', 'websocket'); self.send_header('Connection', 'Upgrade'); self.send_header('Sec-WebSocket-Accept', accept)
        self.end_headers()
        w, r = self.wfile, self.rfile
        self.close_connection = True
        if sid == 'full':
            w.write(ws_frame(8, struct.pack('>H', 4029) + b'too many open terminals')); w.flush(); time.sleep(0.2); return
        if q.get('kind', [''])[0] in ('logs', 'apt', 'winupdate'):
            w.write(ws_frame(2, ('stream kind=%s unit=%s priority=%s kernel=%s lines=%s mode=%s names=%s ids=%s\n' % tuple(q.get(k, [''])[0] for k in ('kind', 'unit', 'priority', 'kernel', 'lines', 'mode', 'names', 'ids'))).encode())); w.flush()
        else:
          w.write(ws_frame(2, ('opened %sx%s kind=%s user=%s\n' % (q.get('cols', ['?'])[0], q.get('rows', ['?'])[0], q.get('kind', ['?'])[0], q.get('user', [''])[0])).encode())); w.flush()
        try:
            while True:
                op, data = ws_read(r)
                if op == 8: w.write(ws_frame(8, data[:2])); w.flush(); return
                if op == 9: w.write(ws_frame(10, data)); w.flush(); continue
                if op == 2:
                    if data == b'exit\n':
                        w.write(ws_frame(1, b'{"closed":"exit"}')); w.write(ws_frame(8, struct.pack('>H', 1000))); w.flush(); return
                    w.write(ws_frame(2, b'echo:' + data)); w.flush()
                elif op == 1:
                    m = json.loads(data)
                    if 'resize' in m:
                        w.write(ws_frame(2, ('resized %sx%s\n' % (m['resize']['cols'], m['resize']['rows'])).encode())); w.flush()
        except EOFError:
            return

    def server_page(self, p):
        # the page of a server: its detail, the history of its metrics, the files of a folder, a download
        def ok(fn, *a, **k):
            fn(*a, **k)
            return True
        u = urlparse(p)
        parts = u.path.split('/')
        q = parse_qs(u.query)
        if len(parts) == 4:
            if parts[3] == 'missing': return ok(self.err, 404, 'server not found')
            return ok(self.send, 200, json.dumps({'id': parts[3], 'name': 'web-01', 'hostname': 'web-01', 'address': '10.0.1.21', 'os': 'Debian GNU/Linux 12 (bookworm)', 'agent_version': '1.1.11',
                'online': True, 'modules': ['terminal', 'files'], 'tag_items': [{'id': 1, 'name': 'web', 'color': 'blue'}], 'note': 'main web',
                'system': {'hostname': 'web-01', 'os': 'Debian GNU/Linux 12', 'kernel': '6.1.0-18-amd64', 'arch': 'amd64', 'cpu_model': 'Xeon', 'cpu_cores': 4, 'mem_total': 8589934592},
                'sample': {'cpu': 41.5, 'mem_used': 4294967296, 'mem_total': 8589934592, 'mem_pct': 50, 'disks': [{'mount': '/', 'total': 214748364800, 'used': 88046829568, 'pct': 41}], 'disk_pct': 41,
                           'rx_bps': 924000, 'tx_bps': 408000, 'load': [0.62, 0.48, 0.51], 'uptime': 3500000, 'procs': 212, 'failed_services': 1, 'reboot_required': True, 'updates': 3, 'security_updates': 1, 'image_updates': 2},
                'spark': [10, 20, 30.5]}).encode())
        if len(parts) == 5 and parts[4] == 'metrics':
            period = q.get('period', ['24h'])[0]
            if period not in ('1h', '24h', '7d', '30d'): return ok(self.err, 400, 'unknown period')
            pts = [{'ts': 1000 + i * 60, 'cpu_avg': 10 + i, 'cpu_max': 20 + i, 'mem_avg': 50, 'mem_max': 55, 'disk_avg': 41, 'disk_max': 41, 'rx_avg': 1000 * i, 'rx_max': 2000 * i, 'tx_avg': 500 * i, 'tx_max': 900 * i} for i in range(5)]
            return ok(self.send, 200, json.dumps({'period': period, 'points': pts, 'stats': {'cpu': {'min': 10, 'avg': 12, 'p95': 14, 'max': 20}}, 'events': [{'ts': 1100, 'kind': 'reboot', 'message': 'rebooted'}]}).encode())
        if len(parts) == 5 and parts[4] == 'files':
            path = q.get('path', ['/'])[0]
            if path == '/denied': return ok(self.err, 403, 'forbidden')
            if path == '/': ents = [{'name': 'var', 'type': 'dir', 'size': 4096, 'mode_str': 'drwxr-xr-x', 'owner': 'root', 'group': 'root', 'mtime': 1700000000}, {'name': 'a.txt', 'type': 'file', 'size': 12, 'mode_str': '-rw-r--r--', 'owner': 'root', 'group': 'root', 'mtime': 1700000100}, {'name': 'Docs', 'type': 'dir', 'size': 4096, 'mode_str': 'drwxr-xr-x', 'owner': 'root', 'group': 'root', 'mtime': 1700000200}, {'name': 'link', 'type': 'link', 'size': 3, 'mode_str': 'lrwxrwxrwx', 'owner': 'root', 'group': 'root', 'mtime': 1, 'link_target': '/var'}]
            else: ents = [{'name': 'b & c=d.txt', 'type': 'file', 'size': 5, 'mode_str': '-rw-r--r--', 'owner': 'u', 'group': 'g', 'mtime': 2}]
            return ok(self.send, 200, json.dumps({'path': path, 'entries': ents}).encode())
        if len(parts) == 5 and parts[4] == 'services':
            return ok(self.send, 200, json.dumps({'services': [
                {'name': 'nginx.service', 'description': 'A high performance web server', 'active': 'active', 'sub': 'running', 'enabled': 'enabled'},
                {'name': 'sshd.service', 'description': 'OpenBSD Secure Shell server', 'active': 'active', 'sub': 'running', 'enabled': 'enabled'},
                {'name': 'cron.service', 'description': 'Regular background program processing daemon', 'active': 'failed', 'sub': 'failed', 'enabled': 'enabled'},
                {'name': 'old.service', 'description': '', 'active': 'inactive', 'sub': 'dead', 'enabled': 'disabled'}]}).encode())
        if len(parts) == 5 and parts[4] == 'processes':
            sort = q.get('sort', ['cpu'])[0]
            procs = [{'pid': 10, 'user': 'root', 'cpu': 1.5, 'mem_pct': 40.0, 'rss': 3000000, 'state': 'S', 'cmd': '/usr/sbin/nginx -g daemon off;'},
                     {'pid': 20, 'user': 'www', 'cpu': 9.5, 'mem_pct': 2.0, 'rss': 100000, 'state': 'R', 'cmd': 'python3 app.py'},
                     {'pid': 30, 'user': 'root', 'cpu': 0.1, 'mem_pct': 0.5, 'rss': 5000, 'state': 'S', 'cmd': 'bash'}]
            key = {'cpu': lambda p: -p['cpu'], 'mem': lambda p: -p['mem_pct'], 'name': lambda p: p['cmd']}.get(sort, lambda p: -p['cpu'])
            return ok(self.send, 200, json.dumps({'processes': sorted(procs, key=key), 'total': 212}).encode())
        if len(parts) == 5 and parts[4] == 'apt':
            return ok(self.send, 200, json.dumps({'packages': [{'name': 'openssl', 'current': '3.0.1', 'candidate': '3.0.2', 'security': True}, {'name': 'vim', 'current': '9.0', 'candidate': '9.1', 'ignored': True}, {'name': 'linux-image', 'current': '6.1.0', 'candidate': '6.1.1', 'reboot': True}], 'reboot_required': True, 'last_check': 1700000000}).encode())
        if len(parts) == 5 and parts[4] == 'docker':
            if parts[3] == 'nodocker': return ok(self.send, 200, b'{"available":false}')
            return ok(self.send, 200, json.dumps({'available': True, 'containers': [
                {'id': 'abc123', 'name': 'web', 'image': 'nginx:1.25', 'state': 'running', 'status': 'Up 2 days', 'ports': ['80->8080/tcp'], 'cpu': 1.2, 'mem_used': 50000000, 'mem_limit': 500000000, 'upd': 'new'},
                {'id': 'def456', 'name': 'db', 'image': 'postgres:16', 'state': 'exited', 'status': 'Exited (0) 3 hours ago', 'ports': [], 'cpu': 0, 'mem_used': 0, 'mem_limit': 0}],
                'images': [{'id': 'sha256:aaa', 'repo_tags': ['nginx:1.25'], 'size': 190000000, 'used': True}, {'id': 'sha256:bbbbbbbbbbbbbbbb', 'repo_tags': [], 'size': 5000, 'used': False}]}).encode())
        if len(parts) == 5 and parts[4] == 'disk':
            return ok(self.send, 200, json.dumps({'filesystems': [{'mount': '/', 'device': '/dev/sda1', 'fs': 'ext4', 'total': 100000000000, 'used': 41000000000, 'avail': 59000000000, 'pct': 41, 'days_left': 120, 'per_day': 500000000}], 'cleanup': [{'kind': 'journal', 'size': 800000000, 'frees': 700000000}, {'kind': 'apt', 'size': 200000000, 'frees': 200000000}]}).encode())
        if len(parts) == 6 and parts[4] == 'files' and parts[5] == 'content':
            path = q.get('path', [''])[0]
            if path == '/denied': return ok(self.err, 403, 'forbidden')
            if path == '/bin.dat': return ok(self.err, 415, 'not a text file')
            if path == '/huge.log': return ok(self.err, 413, 'file too large')
            if path == '/gone': return ok(self.err, 404, 'not found')
            text = TEXTS.get(path, 'hello\nworld\n')
            return ok(self.send, 200, json.dumps({'path': path, 'content': text, 'size': len(text.encode()), 'mtime': MTIMES.get(path, 1700000000000), 'mode': '0644'}).encode())
        if len(parts) == 6 and parts[4] == 'files' and parts[5] == 'stat':
            path = q.get('path', [''])[0]
            if path == '/gone': return ok(self.err, 404, 'not found')
            prot = path in ('/etc', '/usr')
            return ok(self.send, 200, json.dumps({'name': path.rsplit('/', 1)[-1] or '/', 'path': path, 'type': 'dir' if prot else 'file', 'size': 12, 'mode': '4755' if path == '/suid' else '0644', 'mode_str': '-rw-r--r--', 'owner': 'root', 'group': 'adm', 'mtime': 1700000000, 'mime': 'text/plain', 'protected': prot}).encode())
        if len(parts) == 6 and parts[4] == 'files' and parts[5] == 'download':
            path = q.get('path', [''])[0]
            if path == '/denied': return ok(self.err, 403, 'forbidden')
            body = ('content of ' + path).encode()
            self.send_response(200); self.send_header('Content-Type', 'application/octet-stream'); self.send_header('Content-Length', str(len(body))); self.end_headers(); self.wfile.write(body); return True
        return False

    def desktop(self, p):
        # the remote desktop: a WebSocket of binary messages (RDCleanPath in the real one); here every message comes back with a prefix
        u = urlparse(p)
        sid = u.path.split('/')[3]
        if sid == 'off': return self.err(503, 'agent is offline')
        key = self.headers.get('Sec-WebSocket-Key')
        accept = base64.b64encode(hashlib.sha1((key + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').encode()).digest()).decode()
        self.send_response(101)
        self.send_header('Upgrade', 'websocket'); self.send_header('Connection', 'Upgrade'); self.send_header('Sec-WebSocket-Accept', accept)
        self.end_headers()
        w, r = self.wfile, self.rfile
        self.close_connection = True
        if sid == 'full':
            w.write(ws_frame(8, struct.pack('>H', 4030) + b'a remote desktop of this server is already open')); w.flush(); time.sleep(0.2); return
        login = parse_qs(u.query).get('login', [''])[0]
        w.write(ws_frame(2, b'hello:' + login.encode())); w.flush()
        try:
            while True:
                op, data = ws_read(r)
                if op == 8: w.write(ws_frame(8, data[:2])); w.flush(); return
                if op == 9: w.write(ws_frame(10, data)); w.flush(); continue
                if op == 2:
                    if data == b'drop':
                        w.write(ws_frame(8, struct.pack('>H', 1011) + b'cannot open the remote desktop')); w.flush(); return
                    w.write(ws_frame(2, b'echo:' + data)); w.flush()
        except EOFError:
            return

    def assistant(self, p):
        u = urlparse(p)
        sid = u.path.split('/')[3]
        q = parse_qs(u.query)
        if sid == 'off': return self.err(503, 'agent is offline')
        if sid == 'denied': return self.err(403, 'forbidden')
        key = self.headers.get('Sec-WebSocket-Key')
        accept = base64.b64encode(hashlib.sha1((key + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').encode()).digest()).decode()
        self.send_response(101)
        self.send_header('Upgrade', 'websocket'); self.send_header('Connection', 'Upgrade'); self.send_header('Sec-WebSocket-Accept', accept)
        self.end_headers()
        w, r = self.wfile, self.rfile
        self.close_connection = True
        if sid == 'full':
            w.write(ws_frame(8, struct.pack('>H', 4031) + b'this session is already open in the Assistant')); w.flush(); time.sleep(0.2); return
        def frame(t, payload): w.write(ws_frame(2, bytes([t]) + payload)); w.flush()
        frame(1, json.dumps({'w': 64, 'h': 48, 'session': 1, 'user': 'admin', 'monitors': [{'x': 0, 'y': 0, 'w': 64, 'h': 48, 'primary': True}, {'x': 64, 'y': 0, 'w': 0, 'h': 0}], 'monitor': -1}).encode())
        frame(2, struct.pack('>H', 1) + struct.pack('>HHHHI', 4, 4, 16, 16, len(TILE)) + TILE)
        frame(3, json.dumps({'fps': 14.5, 'kbps': 800, 'q': 80, 'desktop': 'winlogon'}).encode())
        try:
            while True:
                op, data = ws_read(r)
                if op == 8: w.write(ws_frame(8, data[:2])); w.flush(); return
                if op == 9: w.write(ws_frame(10, data)); w.flush(); continue
                if op == 2 and data:
                    body = data[1:].decode('utf-8', 'replace')
                    if data[0] == 18 and '"bye"' in body:
                        frame(4, json.dumps({'reason': 'done', 'reconnect': True}).encode()); w.write(ws_frame(8, struct.pack('>H', 1000))); w.flush(); return
                    frame(5, json.dumps({'text': '%d:%s' % (data[0], body)}).encode())
                    if data[0] == 22: frame(7, json.dumps({'ok': True}).encode())
        except EOFError:
            return

    def mgmt_post(self, parts, body):
        if not self.authed(): return self.err(401, 'unauthorized')
        if self.headers.get('X-CSRF-Token') != CSRF: return self.err(403, 'bad csrf token')
        def need(): return self.send(409, json.dumps({'error': 'confirmation required', 'dangerous': True}).encode())
        kind = parts[4]
        if kind == 'win' and parts[5] == 'winupdate' and parts[6] == 'list':
            return self.send(200, json.dumps({'updates': [{'id': '11111111-1111-1111-1111-111111111111', 'title': '2024-05 Cumulative Update (KB5037771)', 'kb': 'KB5037771', 'size': 650000000, 'security': True, 'downloaded': False, 'need_reboot': True}, {'id': '22222222-2222-2222-2222-222222222222', 'title': 'Defender definition update', 'kb': 'KB2267602', 'size': 2000000, 'security': False, 'downloaded': True}], 'reboot_pending': True}).encode())
        if kind == 'reboot':
            if not body.get('confirm'): return need()
            OPS.append(['reboot', parts[3]])
            return self.send(200, b'{"ok":true}')
        if kind == 'services':
            unit, action = parts[5], parts[6]
            if unit == 'sshd.service' and action == 'stop' and not body.get('confirm'): return need()
            OPS.append(['service', unit, action, bool(body.get('confirm'))])
            return self.send(200, b'{"ok":true}')
        if kind == 'processes':
            if int(parts[5]) <= 1: return self.err(400, 'invalid pid')
            OPS.append(['kill', int(parts[5]), body.get('signal')])
            return self.send(200, b'{"ok":true}')
        if kind == 'disk':
            OPS.append(['clean', body.get('kind')])
            return self.send(200, json.dumps({'ok': True, 'freed': 700000000, 'output': 'done'}).encode())
        if kind == 'docker':
            cid, action = parts[6], parts[7]
            if action == 'remove' and not body.get('confirm'): return need()
            OPS.append(['docker', cid, action, bool(body.get('confirm'))])
            return self.send(200, b'{"ok":true}')

    def do_DELETE(self):
        n = int(self.headers.get('Content-Length') or 0)
        body = json.loads(self.rfile.read(n) or b'{}')
        if not self.authed(): return self.err(401, 'unauthorized')
        if self.headers.get('X-CSRF-Token') != CSRF: return self.err(403, 'bad csrf token')
        parts = self.path.split('/')
        if len(parts) == 4 and parts[1] == 'api' and parts[2] == 'servers':
            if body.get('name') != 'web-01': return self.err(422, 'confirmation name does not match')
            OPS.append(['delete', parts[3], body.get('name'), bool(body.get('uninstall'))])
            return self.send(200, json.dumps({'deleted': True, 'uninstall_sent': False, 'was_online': True}).encode())
        self.err(404, 'not found')

    def do_PUT(self):
        n = int(self.headers.get('Content-Length') or 0)
        body = json.loads(self.rfile.read(n) or b'{}')
        if not self.authed(): return self.err(401, 'unauthorized')
        if self.headers.get('X-CSRF-Token') != CSRF: return self.err(403, 'bad csrf token')
        if self.path.endswith('/files/content'):
            path = body.get('path')
            if path == '/denied': return self.err(403, 'forbidden')
            if body.get('mtime') != MTIMES.get(path, 1700000000000): return self.err(409, 'file changed')
            TEXTS[path] = body.get('content', '')
            MTIMES[path] = MTIMES.get(path, 1700000000000) + 1000
            OPS.append(['save', path, body.get('content', '')])
            return self.send(200, json.dumps({'ok': True, 'mtime': MTIMES[path]}).encode())
        if self.path.endswith('/assistant/settings'):
            SETTINGS.clear(); SETTINGS.update(body)
            return self.send(200, json.dumps({'settings': body, 'applied': 0}).encode())
        self.err(404, 'not found')

    def files_post(self, op, body):
        if not self.authed(): return self.err(401, 'unauthorized')
        if self.headers.get('X-CSRF-Token') != CSRF: return self.err(403, 'bad csrf token')
        path = body.get('path', '')
        if path == '/denied' or body.get('dir') == '/denied': return self.err(403, 'forbidden')
        if op in ('create', 'mkdir') and path.endswith('/exists'): return self.err(409, 'already exists')
        if op in ('copy', 'move') and body.get('dir') == '/taken' and body.get('mode') == 'fail': return self.err(409, 'already exists')
        if op in ('delete', 'chmod', 'chown', 'move') and path == '/etc' and not body.get('confirm'): return self.err(409, 'confirmation required')
        OPS.append([op, body])
        return self.send(200, json.dumps({'ok': True, 'name': 'copy of ' + path.rsplit('/', 1)[-1]}).encode())

    def do_POST(self):
        n = int(self.headers.get('Content-Length') or 0)
        raw = self.rfile.read(n)
        if self.path.startswith('/api/servers/') and '/files/upload?' in self.path:
            if not self.authed(): return self.err(401, 'unauthorized')
            if self.headers.get('X-CSRF-Token') != CSRF: return self.err(403, 'bad csrf token')
            path = parse_qs(urlparse(self.path).query).get('path', [''])[0]
            if path.endswith('/exists'): return self.err(409, 'already exists')
            OPS.append(['upload', path, len(raw), self.headers.get('Content-Type')])
            return self.send(200, b'{"ok":true}')
        body = json.loads(raw or b'{}')
        parts = self.path.split('/')
        if self.path.startswith('/api/servers/') and len(parts) >= 6 and parts[4] in ('services', 'processes', 'disk', 'docker', 'win') or (len(parts) == 5 and parts[4] == 'reboot' and self.path.startswith('/api/servers/')):
            return self.mgmt_post(parts, body)
        if self.path.startswith('/api/servers/') and len(parts) == 6 and parts[4] == 'files' and parts[5] in ('create', 'mkdir', 'delete', 'rename', 'copy', 'move', 'chmod', 'chown'):
            return self.files_post(parts[5], body)
        if self.path == '/api/login':
            login, pw, totp = body.get('login'), body.get('password'), body.get('totp')
            dev = self.headers.get('X-Linager-Device', '')
            SEEN.append([dev, self.headers.get('X-Linager-Client', '')])
            if pw != 'secret' or login not in ('admin', 'twofa'): return self.err(401, 'invalid credentials')
            if login == 'twofa':
                if not totp: return self.err(401, 'totp_required')
                if totp != '123456': return self.err(401, 'invalid credentials')
            cookie = 'linager_session=%s; Path=/; HttpOnly; Secure; SameSite=Strict; Max-Age=604800' % TOKEN
            if dev.startswith('D'): return self.err(403, 'device_denied')
            out = {'csrf': CSRF, 'login': login, 'role': 'admin', 'totp_enabled': login == 'twofa'}
            if device_body(dev) is not None: out['device'] = device_body(dev)
            return self.send(200, json.dumps(out).encode(), extra=[('Set-Cookie', cookie)])
        if self.path.startswith('/api/snippets/') and self.path.endswith('/ran'):
            if not self.authed(): return self.err(401, 'unauthorized')
            if self.headers.get('X-CSRF-Token') != CSRF: return self.err(403, 'bad csrf token')
            RAN.append((self.path.split('/')[3], body.get('server_id')))
            return self.send(200, b'{"ok":true}')
        if self.path == '/api/test/approve':
            APPROVED[0] = True
            return self.send(200, b'{"ok":true}')
        if self.path == '/api/ran':
            return self.send(200, json.dumps(RAN).encode())
        if self.path == '/api/logout':
            if not self.authed(): return self.err(401, 'unauthorized')
            if self.headers.get('X-CSRF-Token') != CSRF: return self.err(403, 'bad csrf token')
            return self.send(200, b'{"ok":true}', extra=[('Set-Cookie', 'linager_session=; Path=/; Max-Age=0; HttpOnly; SameSite=Strict')])
        self.err(404, 'not found')

TEXTS = {}
MTIMES = {}
OPS = []

srv = ThreadingHTTPServer(('127.0.0.1', 0), H)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain(cert, key)
srv.socket = ctx.wrap_socket(srv.socket, server_side=True)
print(srv.server_address[1], flush=True)
srv.serve_forever()
