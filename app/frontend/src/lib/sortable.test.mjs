// Run with: node --test src/lib/sortable.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { dropIndex } from './sortable.ts';

// the middles of the other rows, top to bottom
const mids = [20, 60, 100];

test('a row dropped above the first middle goes first, below the last goes last', () => {
  assert.equal(dropIndex(mids, 5), 0);
  assert.equal(dropIndex(mids, 20), 0); // exactly on a middle: before that row
  assert.equal(dropIndex(mids, 61), 2);
  assert.equal(dropIndex(mids, 500), 3);
});

test('an empty list takes the row at 0', () => {
  assert.equal(dropIndex([], 40), 0);
});
