// Run with: node --test src/lib/server/sidebar-dots.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const src = (f) => readFileSync(new URL(f, import.meta.url), 'utf8');

test('in a folded sidebar the counters of updates, images and failed services stay as dots of their colour', () => {
  const css = src('../../app.css');
  // the counters are no longer hidden when the sidebar is folded...
  const hide = css.match(/\.sp\.collapsed \.sp-id\{[^\n]*/)?.[0] ?? '';
  assert.doesNotMatch(hide, /\.si \.cnt/);
  // ...they become a dot on the icon
  assert.match(css, /\.sp\.collapsed \.si \.cnt\{position:absolute;[^}]*width:8px;height:8px;[^}]*border-radius:50%;[^}]*font-size:0/);
  const page = src('./ServerPage.svelte');
  assert.match(page, /it\.key === 'updates'[^\n]*srv\.nav\.badgeUpdates/); // the dot says what it is
  assert.match(page, /it\.key === 'docker'[^\n]*srv\.nav\.badgeImages/);
  assert.match(page, /it\.key === 'services'[^\n]*cnt crit/); // red for failed services
});
