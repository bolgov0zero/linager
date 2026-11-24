// Run with: node --test src/lib/no-ellipsis.test.mjs
// The names of menu items, buttons and fields have no trailing dots ("Move to…", "Delete…"). A text that says that something is
// going on ("Loading…", "Connecting…") may keep them: this test only watches the names listed here, so a new name with dots is
// caught by adding it to the list of what is checked: any text under these prefixes must be clean.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const load = (f) => JSON.parse(readFileSync(new URL(`./locales/${f}`, import.meta.url), 'utf8'));
// keys of names that had dots and lost them, and the groups of keys whose texts are names of actions
const NAMES = ['docker.act.edit', 'usr.menu.delete', 'usr.props.delete', 'net.config.apply', 'assist.quality', 'keys.file.pick', 'keys.f.bind.change', 'keys.pick.ph', 'wu.setPassword', 'wu.editGroups', 'term.snippets.manage', 'keys.unlock.other', 'fld.menu.move', 'fld.menu.delete', 'dash.menu.folder', 'pult.search', 'win.note.ph'];
const PREFIXES = ['fld.menu.', 'fld.add', 'fld.move'];

for (const lang of ['ru.json', 'en.json']) {
  test(`${lang}: the names of actions have no trailing dots`, () => {
    const d = load(lang);
    const bad = [];
    for (const [k, v] of Object.entries(d)) {
      if (!NAMES.includes(k) && !PREFIXES.some((p) => k.startsWith(p))) continue;
      if (/(…|\.\.\.)\s*$/.test(v)) bad.push(`${k}: ${v}`);
    }
    assert.deepEqual(bad, []);
    for (const k of NAMES) assert.ok(k in d, `${k} exists`);
  });
}
