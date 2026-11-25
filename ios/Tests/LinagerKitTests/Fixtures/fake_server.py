#!/usr/bin/env python3
"""A tiny stand-in for the application port of the Linager server, for the tests of LinagerKit.
usage: fake_server.py CERT KEY DASHBOARD.json GROUPS.json  (prints the port, then serves until killed)"""
import json, ssl, sys, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

cert, key, dash, groups = sys.argv[1:5]
DASH = open(dash, 'rb').read()
GROUPS = open(groups, 'rb').read()
TOKEN, CSRF = 'tok123', 'csrf-abc'

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
        if p == '/deny': return self.send(403, b'forbidden\n', 'text/plain')
        if not self.authed(): return self.err(401, 'unauthorized')
        if p == '/api/me': return self.send(200, json.dumps({'csrf': CSRF, 'login': 'admin', 'role': 'admin', 'totp_enabled': False}).encode())
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
    def do_POST(self):
        n = int(self.headers.get('Content-Length') or 0)
        body = json.loads(self.rfile.read(n) or b'{}')
        if self.path == '/api/login':
            login, pw, totp = body.get('login'), body.get('password'), body.get('totp')
            if pw != 'secret' or login not in ('admin', 'twofa'): return self.err(401, 'invalid credentials')
            if login == 'twofa':
                if not totp: return self.err(401, 'totp_required')
                if totp != '123456': return self.err(401, 'invalid credentials')
            cookie = 'linager_session=%s; Path=/; HttpOnly; Secure; SameSite=Strict; Max-Age=604800' % TOKEN
            return self.send(200, json.dumps({'csrf': CSRF, 'login': login, 'role': 'admin', 'totp_enabled': login == 'twofa'}).encode(), extra=[('Set-Cookie', cookie)])
        if self.path == '/api/logout':
            if not self.authed(): return self.err(401, 'unauthorized')
            if self.headers.get('X-CSRF-Token') != CSRF: return self.err(403, 'bad csrf token')
            return self.send(200, b'{"ok":true}', extra=[('Set-Cookie', 'linager_session=; Path=/; Max-Age=0; HttpOnly; SameSite=Strict')])
        self.err(404, 'not found')

srv = ThreadingHTTPServer(('127.0.0.1', 0), H)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain(cert, key)
srv.socket = ctx.wrap_socket(srv.socket, server_side=True)
print(srv.server_address[1], flush=True)
srv.serve_forever()
