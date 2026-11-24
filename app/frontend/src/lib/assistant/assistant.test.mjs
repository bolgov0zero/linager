// Run with: node --test src/lib/assistant/assistant.test.mjs (Node 22.18 or newer strips the types by itself).
import test from 'node:test';
import assert from 'node:assert/strict';
import { CLIP_MAX, MSG_TYPE, TYPING_MAX, WheelNotches, cleanMonitors, encodeClipGet, encodeClipSet, encodeInput, encodeMonitor, encodeTyping, isSecureDesktop, isSignInDesktop, parseTiles } from './protocol.ts';
import { scanKey } from './scancodes.ts';
import { PRESETS, estimateMbit, presetOf } from './quality.ts';

function tilesPayload(tiles) {
  const parts = [Buffer.from([tiles.length >> 8, tiles.length & 255])];
  for (const t of tiles) {
    const head = Buffer.alloc(12);
    head.writeUInt16BE(t.x, 0);
    head.writeUInt16BE(t.y, 2);
    head.writeUInt16BE(t.w, 4);
    head.writeUInt16BE(t.h, 6);
    head.writeUInt32BE(t.jpeg.length, 8);
    parts.push(head, Buffer.from(t.jpeg));
  }
  return new Uint8Array(Buffer.concat(parts));
}

test('tiles are read in order with their position and bytes', () => {
  const data = tilesPayload([
    { x: 0, y: 0, w: 64, h: 64, jpeg: [1, 2, 3] },
    { x: 64, y: 128, w: 32, h: 16, jpeg: [] },
    { x: 300, y: 7, w: 5, h: 6, jpeg: [9] },
  ]);
  const tiles = parseTiles(data);
  assert.equal(tiles.length, 3);
  assert.deepEqual([tiles[1].x, tiles[1].y, tiles[1].w, tiles[1].h], [64, 128, 32, 16]);
  assert.deepEqual([...tiles[0].jpeg], [1, 2, 3]);
  assert.equal(tiles[1].jpeg.length, 0);
  assert.equal(tiles[2].x, 300);
});

test('a cut or lying payload is refused, not read past its end', () => {
  const ok = tilesPayload([{ x: 1, y: 2, w: 3, h: 4, jpeg: [1, 2, 3, 4] }]);
  assert.equal(parseTiles(ok.subarray(0, ok.length - 1)), null);
  assert.equal(parseTiles(ok.subarray(0, 7)), null);
  assert.equal(parseTiles(new Uint8Array([0])), null);
  assert.deepEqual(parseTiles(new Uint8Array([0, 0])), []);
  const lie = Uint8Array.from(ok);
  lie[2 + 8 + 3] = 0xff; // the length says 255 bytes more than there are
  assert.equal(parseTiles(lie), null);
});

test('an input message is the type byte and JSON, and stays under the limit of the panel', () => {
  const m = encodeInput({ t: 'k', sc: 0x1e, ext: false, d: true });
  assert.equal(m[0], 16);
  assert.deepEqual(JSON.parse(new TextDecoder().decode(m.subarray(1))), { t: 'k', sc: 30, ext: false, d: true });
  const big = encodeInput({ t: 'b', b: 4, d: true, x: 65535, y: 65535 });
  assert.ok(big.length - 1 <= 256);
});

test('the wheel gives whole notches, keeps the rest, and is clamped', () => {
  const w = new WheelNotches();
  assert.deepEqual(w.push(0, 100, 0), { dx: 0, dy: 1 });
  assert.deepEqual(w.push(0, -250, 0), { dx: 0, dy: -2 });
  assert.deepEqual(w.push(0, -250, 0), { dx: 0, dy: -3 }); // the half notch that was left comes with the next one
  assert.deepEqual(w.push(30, 0, 0), { dx: 0, dy: 0 });
  assert.deepEqual(w.push(80, 0, 0), { dx: 1, dy: 0 });
  assert.deepEqual(new WheelNotches().push(0, 3, 1), { dx: 0, dy: 1 });
  assert.deepEqual(new WheelNotches().push(0, 99999, 0), { dx: 0, dy: 10 });
  assert.deepEqual(new WheelNotches().push(0, -99999, 0), { dx: 0, dy: -10 });
});

test('keys map to set-1 scan codes with the extended flag', () => {
  assert.deepEqual(scanKey('KeyA'), { sc: 0x1e, ext: false });
  assert.deepEqual(scanKey('Enter'), { sc: 0x1c, ext: false });
  assert.deepEqual(scanKey('NumpadEnter'), { sc: 0x1c, ext: true });
  assert.deepEqual(scanKey('ControlRight'), { sc: 0x1d, ext: true });
  assert.deepEqual(scanKey('AltRight'), { sc: 0x38, ext: true });
  assert.deepEqual(scanKey('MetaLeft'), { sc: 0x5b, ext: true });
  assert.deepEqual(scanKey('MetaRight'), { sc: 0x5c, ext: true });
  assert.deepEqual(scanKey('ContextMenu'), { sc: 0x5d, ext: true });
  assert.deepEqual(scanKey('ArrowLeft'), { sc: 0x4b, ext: true });
  assert.deepEqual(scanKey('Numpad4'), { sc: 0x4b, ext: false });
  assert.deepEqual(scanKey('F12'), { sc: 0x58, ext: false });
  assert.equal(scanKey('MediaPlayPause'), null);
});

test('every scan code is 1..255 and no two plain keys share one', () => {
  const seen = new Map();
  for (const code of ['Escape', 'Digit0', 'Minus', 'Tab', 'KeyQ', 'Space', 'F10', 'F11', 'Numpad0', 'NumpadAdd', 'IntlBackslash', 'ShiftLeft', 'ShiftRight']) {
    const k = scanKey(code);
    assert.ok(k.sc >= 1 && k.sc <= 255, code);
    assert.ok(!seen.has(k.sc), `${code} and ${seen.get(k.sc)} share ${k.sc}`);
    seen.set(k.sc, code);
  }
});

const body = (m) => JSON.parse(new TextDecoder().decode(m.subarray(1)));

test('the monitor message carries the index, -1 for all of them', () => {
  const m = encodeMonitor(1);
  assert.equal(m[0], 20);
  assert.deepEqual(body(m), { i: 1 });
  assert.deepEqual(body(encodeMonitor(-1)), { i: -1 });
});

test('the clipboard messages: ClipSet is JSON with the text, ClipGet is the type byte alone', () => {
  const set = encodeClipSet('привет\nworld');
  assert.equal(set[0], 18);
  assert.deepEqual(body(set), { text: 'привет\nworld' });
  assert.deepEqual([...encodeClipGet()], [19]);
});

test('a text over the limit of ClipSet is refused, one at the limit is not', () => {
  assert.equal(encodeClipSet('a'.repeat(CLIP_MAX)), null); // the JSON around the text goes over
  assert.ok(encodeClipSet('a'.repeat(CLIP_MAX - 20)));
  assert.equal(encodeClipSet('ы'.repeat(CLIP_MAX / 2)), null); // two bytes a letter
  assert.ok(encodeClipSet(''));
});

test('the monitors of a Hello are cleaned: bad entries dropped, missing list is empty', () => {
  assert.deepEqual(cleanMonitors(undefined), []);
  assert.deepEqual(cleanMonitors('x'), []);
  const list = cleanMonitors([
    { x: 0, y: 0, w: 1920, h: 1080, primary: true },
    { x: 1920, y: -100, w: 2560, h: 1440 },
    { x: 0, y: 0, w: 0, h: 10, primary: false },
    null,
    5,
  ]);
  assert.equal(list.length, 2);
  assert.deepEqual(list[1], { x: 1920, y: -100, w: 2560, h: 1440, primary: false });
  assert.equal(list[0].primary, true);
});

test('only a desktop other than the default (and not empty) is a secure one', () => {
  assert.equal(isSecureDesktop('default'), false);
  assert.equal(isSecureDesktop(''), false);
  assert.equal(isSecureDesktop(undefined), false);
  assert.equal(isSecureDesktop('winlogon'), true);
  assert.equal(isSecureDesktop('screen-saver'), true);
});

test('a preset is recognised, with or without the pointer, and a change makes it custom', () => {
  assert.equal(presetOf({ ...PRESETS.normal }), 'normal');
  assert.equal(presetOf({ ...PRESETS.hq, show_cursor: true }), 'hq');
  assert.equal(presetOf({ ...PRESETS.eco, fps: 9 }), 'custom');
  // the limit does not matter while there is none
  assert.equal(presetOf({ ...PRESETS.normal, nolimit: true, mbit: 17 }), 'custom');
});

test('the traffic estimate follows the colours and never exceeds the limit', () => {
  assert.equal(estimateMbit({ ...PRESETS.normal, mbit: 4, fps: 30 }), 4);
  assert.equal(estimateMbit({ ...PRESETS.normal, mbit: 20, fps: 10, colors: '24' }), 5.5);
  assert.ok(estimateMbit({ ...PRESETS.normal, mbit: 20, fps: 10, colors: 'gray' }) < 5.5);
  assert.equal(estimateMbit({ ...PRESETS.normal, nolimit: true, fps: 30, colors: '24' }), 16.5);
});

test('a typing frame carries the text, what comes after and the name of the key', () => {
  const m = encodeTyping('CORP\\ann', 'tab', 'Home Pass');
  assert.equal(m[0], MSG_TYPE);
  assert.deepEqual(JSON.parse(new TextDecoder().decode(m.subarray(1))), { text: 'CORP\\ann', after: 'tab', label: 'Home Pass' });
  const plain = JSON.parse(new TextDecoder().decode(encodeTyping('пароль ✓', '', 'k').subarray(1)));
  assert.equal(plain.after, undefined);
  assert.equal(plain.text, 'пароль ✓');
  // the label loses control characters and is cut at 80
  const l = JSON.parse(new TextDecoder().decode(encodeTyping('x', 'enter', 'a\nb' + 'c'.repeat(100)).subarray(1))).label;
  assert.equal(l.startsWith('a b'), true);
  assert.equal([...l].length, 80);
});

test('a typing that the panel would drop is not made', () => {
  assert.equal(encodeTyping('', 'enter', 'k'), null);
  assert.equal(encodeTyping('a\nb', '', 'k'), null);
  assert.equal(encodeTyping('a'.repeat(TYPING_MAX + 1), '', 'k'), null);
  assert.notEqual(encodeTyping('a'.repeat(TYPING_MAX), '', 'k'), null);
});

test('only the sign-in desktop gets a typing (the screen saver and the usual desktop do not)', () => {
  assert.equal(isSignInDesktop('winlogon'), true);
  assert.equal(isSignInDesktop('default'), false);
  assert.equal(isSignInDesktop('screen-saver'), false);
  assert.equal(isSignInDesktop(undefined), false);
  assert.equal(isSecureDesktop('screen-saver'), true);
});
