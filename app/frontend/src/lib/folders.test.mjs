// Run with: node --test src/lib/folders.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { buildTree, flatten, subtreeIds, subtreesOf, chainOf, pathOf, heightOf, moveProblem, folderOptions } from './folders.ts';

const g = (id, name, parent_id = null) => ({ id, name, parent_id });
// Prod { Web { Front }, DB }, Test { Web }, Ops
const L = [g(1, 'Prod'), g(2, 'Web', 1), g(3, 'Front', 2), g(4, 'DB', 1), g(5, 'Test'), g(6, 'Web', 5), g(7, 'Ops')];

test('the tree is in display order with depth and path', () => {
  const f = flatten(L);
  assert.deepEqual(f.map((n) => `${'-'.repeat(n.depth)}${n.name}`), ['Prod', '-Web', '--Front', '-DB', 'Test', '-Web', 'Ops']);
  assert.equal(f[2].path, 'Prod / Web / Front');
  assert.equal(buildTree(L).length, 3);
});

test('the order of the list decides the order inside a parent', () => {
  const l = [g(1, 'A'), g(2, 'x', 1), g(3, 'y', 1), g(4, 'B')];
  assert.deepEqual(flatten(l).map((n) => n.name), ['A', 'x', 'y', 'B']);
  assert.deepEqual(flatten([l[0], l[2], l[1], l[3]]).map((n) => n.name), ['A', 'y', 'x', 'B']);
});

test('a folder with an unknown parent is not lost, a loop does not hang', () => {
  assert.deepEqual(flatten([g(1, 'A'), g(2, 'lost', 99)]).map((n) => n.name), ['A', 'lost']);
  assert.ok(flatten([g(1, 'a', 2), g(2, 'b', 1)]).length <= 2);
});

test('subtrees and chains', () => {
  assert.deepEqual([...subtreeIds(L, 1)].sort(), [1, 2, 3, 4]);
  assert.deepEqual([...subtreesOf(L, [2, 5])].sort(), [2, 3, 5, 6]);
  assert.deepEqual(chainOf(L, 3), [3, 2, 1]);
  assert.deepEqual(chainOf(L, null), []);
  assert.equal(pathOf(L, 6), 'Test / Web');
  assert.equal(heightOf(L, 1), 3);
});

test('what stops a move: itself, depth, a name in the way', () => {
  assert.equal(moveProblem(L, 1, 3), 'self');
  assert.equal(moveProblem(L, 1, 1), 'self');
  assert.equal(moveProblem(L, 2, 5), 'name'); // Web under Test: Test has a Web
  assert.equal(moveProblem(L, 3, 7), '');
  assert.equal(moveProblem(L, 3, null), '');
  const deep = Array.from({ length: 8 }, (_, i) => g(i + 1, 'n' + i, i ? i : null));
  assert.equal(moveProblem([...deep, g(9, 'x')], 9, 8), 'depth');
  assert.equal(moveProblem([...deep, g(9, 'x')], 9, 7), '');
});

test('the options of a select carry the depth, the path and the icon (an open folder when something lies in it)', () => {
  const o = folderOptions(L, 'No folder');
  assert.deepEqual(o[0], { v: '0', t: 'No folder', i: 'folders' });
  assert.deepEqual(o[3], { v: '3', t: 'Front', d: 2, b: 'Prod / Web / Front', i: 'folder' });
  assert.equal(o[1].i, 'folder-open'); // "Prod" has folders under it
});
