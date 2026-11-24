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

test('the manager shows the Windows sessions on Windows and the SSH ones elsewhere (a Windows agent has no security module)', () => {
  const s = src('ManagerWindow.svelte');
  assert.match(s, /isWindows\(s\.os\)\}<WinSessionsTab \{s\} \/>\{:else\}<SessionsTab \{s\} \/>/);
  const w = src('../server/win/WinSessionsTab.svelte');
  assert.match(w, /winCall<WinUsersState>\(sid, 'users', 'state'\)/);
  assert.match(w, /winCall\(sid, 'users', 'logoff'/);
  assert.doesNotMatch(w, /serverApi2\.security/);
});
