import test from 'node:test';
import assert from 'node:assert/strict';
import { portList, portsShort } from './docker-ports.ts';

test('the same pair on IPv4 and IPv6 is listed once', () => {
  assert.deepEqual(portList(['0.0.0.0:8070->8070/tcp', '[::]:8070->8070/tcp', ':::8070->8070/tcp', '9000/tcp']), ['8070->8070/tcp', '9000/tcp']);
});
test('short form', () => {
  assert.equal(portsShort([]), '—');
  assert.equal(portsShort(null), '—');
  assert.equal(portsShort(['0.0.0.0:80->80/tcp', '[::]:80->80/tcp']), '80->80/tcp');
  assert.equal(portsShort(['0.0.0.0:80->80/tcp', '0.0.0.0:443->443/tcp', '[::]:443->443/tcp']), '80->80/tcp +1');
});
