// Run with: node --test src/lib/desktop/keys.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { planKey } from './keys.ts';

test('the remote desktop keyboard: Cmd of a Mac is Ctrl, clipboard shortcuts are told apart', () => {
  const k = (code, o = {}) => ({ code, ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, ...o });
  assert.equal(planKey(k('MetaLeft', { metaKey: true }), true), 'meta');
  assert.equal(planKey(k('MetaLeft', { metaKey: true }), false), 'pass'); // the Windows key stays itself elsewhere
  assert.equal(planKey(k('KeyV', { metaKey: true }), true), 'paste');
  assert.equal(planKey(k('KeyV', { ctrlKey: true }), false), 'paste');
  assert.equal(planKey(k('KeyC', { metaKey: true }), true), 'copy');
  assert.equal(planKey(k('KeyX', { ctrlKey: true }), false), 'copy');
  assert.equal(planKey(k('KeyA', { metaKey: true }), true), 'chord');
  assert.equal(planKey(k('KeyA', { ctrlKey: true }), true), 'pass');
  assert.equal(planKey(k('KeyA', { metaKey: true }), false), 'pass'); // Cmd is not Ctrl off a Mac
  assert.equal(planKey(k('KeyV', { ctrlKey: true, shiftKey: true }), false), 'pass');
  assert.equal(planKey(k('KeyV', { ctrlKey: true, altKey: true }), false), 'pass'); // AltGr
  assert.equal(planKey(k('ControlLeft', { ctrlKey: true }), true), 'pass');
  assert.equal(planKey(k('KeyV'), true), 'pass');
});

