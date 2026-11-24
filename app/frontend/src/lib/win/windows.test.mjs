// Run with: node --test src/lib/win/windows.test.mjs
// A window that reads the record of its server again and again (`s = await serverApi.detail(id)`) replaces the whole object. Whatever
// reads `s.id` in an effect then runs again, and the Assistant connected again every 20 seconds (the picture blinked). The windows
// must hand the connection the id they were opened with.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const src = (name) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8');

for (const [file, pane] of [['AssistantWindow.svelte', 'AssistantPane'], ['DesktopWindow.svelte', 'DesktopPane']]) {
  test(`${file} gives the pane the id of the window, not a value of the replaced record`, () => {
    const s = src(file);
    assert.match(s, /setInterval/, 'the window polls the record');
    assert.match(s, new RegExp(`<${pane}[^>]*serverId=\\{id\\}`), 'serverId comes from the id of the window');
    assert.doesNotMatch(s, /serverId=\{s\.id\}/);
  });
}
