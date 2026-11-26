<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type Session } from './lib/api';
  import Connect from './lib/Connect.svelte';
  import Dashboard from './lib/Dashboard.svelte';
  import { t } from './lib/i18n.svelte';
  import Login from './lib/Login.svelte';
  import Waiting from './lib/Waiting.svelte';
  import { native, type AppState } from './lib/native';
  import Confirm from './lib/Confirm.svelte';
  import Icon from './lib/Icon.svelte';
  import Console from './lib/console/Console.svelte';
  import { con } from './lib/console/store.svelte';
  import { setMe } from './lib/me.svelte';
  import { nav, route as page } from './lib/route.svelte';
  import ServerPage from './lib/server/ServerPage.svelte';
  import Shell from './lib/Shell.svelte';
  import Terminal from './lib/Terminal.svelte';
  import AssistantWindow from './lib/win/AssistantWindow.svelte';
  import DesktopWindow from './lib/win/DesktopWindow.svelte';
  import EditorWindow from './lib/win/EditorWindow.svelte';
  import FilesWindow from './lib/win/FilesWindow.svelte';
  import ManagerWindow from './lib/win/ManagerWindow.svelte';
  import SettingsWindow from './lib/win/SettingsWindow.svelte';
  import NoteWindow from './lib/win/NoteWindow.svelte';

  // A window of the application is a hash route: #/w/terminal/<server id>?title=<name>. Everything else is the main window.
  function route(): { kind: string; id: string; title: string; arg: string } | null {
    const m = /^#\/w\/(\w+)\/([^?]+)(?:\?(.*))?$/.exec(location.hash);
    if (!m) return null;
    const q = new URLSearchParams(m[3] ?? '');
    return { kind: m[1], id: decodeURIComponent(m[2]), title: q.get('title') ?? m[2], arg: q.get('arg') ?? '' };
  }
  const win = route();

  type Stage = 'loading' | 'connect' | 'login' | 'waiting' | 'main' | 'window' | 'error';
  let stage = $state<Stage>('loading');
  let st = $state<AppState | null>(null);
  let session = $state<Session | null>(null);
  /** Why the sign-in screen is shown instead of the application (a device that was refused or removed). */
  let notice = $state('');

  /** The device signed in but an administrator has not approved it yet. */
  const waiting = (s: Session): boolean => !!s.device?.confirm && s.device.status === 'pending';

  async function enter(s: AppState): Promise<void> {
    st = s;
    if (!s.connected) {
      stage = 'connect';
      return;
    }
    try {
      session = await api.me(); // the cookie of an earlier sign-in may still be alive
      setMe(session);
      stage = waiting(session) ? (win ? 'error' : 'waiting') : win ? 'window' : 'main';
    } catch (e) {
      if (win && e instanceof ApiError && e.status === 401) {
        stage = 'error';
        return;
      }
      if (e instanceof ApiError && e.status === 403 && e.code === 'device_denied') notice = t('login.err.deviceDenied');
      stage = 'login';
    }
  }

  onMount(async () => {
    try {
      let s = await native.state();
      // a session of an earlier run is still kept: open the last server at once, no sign-in screen
      const last = s.profiles[0];
      if (!s.connected && !win && last?.session) s = await native.use(last.id).catch(() => s);
      await enter(s);
    } catch {
      stage = 'error';
    }
  });

  $effect(() => {
    if (nav.section === 'console') con.seen = true;
  });

  async function logout(): Promise<void> {
    try {
      await api.logout();
    } finally {
      session = null;
      st = await native.disconnect();
      stage = 'connect';
    }
  }
</script>

{#if stage === 'loading'}
  <div class="center"><span class="muted">{t('dash.loading')}</span></div>
{:else if stage === 'error'}
  <div class="center"><span class="muted">{t('conn.err.generic')}</span></div>
{:else if stage === 'connect' && st}
  <Connect app={st} onconnected={(s) => void enter(s)} />
{:else if stage === 'login' && st?.profile}
  <Login profile={st.profile} {notice} onsignedin={(s) => { notice = ''; session = s; setMe(s); stage = waiting(s) ? 'waiting' : 'main'; }} onchange={(s) => void enter(s)} />
{:else if stage === 'waiting' && st}
  <Waiting
    name={st.profile?.name ?? ''}
    onapproved={(s) => { session = s; setMe(s); stage = 'main'; }}
    onlogout={logout}
    onlost={(why) => { notice = why === 'denied' ? t('login.err.deviceDenied') : t('wait.ended'); session = null; stage = 'login'; }}
  />
{:else if stage === 'window' && win && win.kind === 'terminal'}
  <Terminal id={win.id} title={win.title} nativeWindows={st?.native ?? false} initialUser={win.arg} />
{:else if stage === 'window' && win && win.kind === 'note'}
  <NoteWindow id={win.id} title={win.title} />
{:else if stage === 'window' && win && win.kind === 'files'}
  <FilesWindow id={win.id} title={win.title} nativeWindows={st?.native ?? false} start={win.arg} />
{:else if stage === 'window' && win && win.kind === 'manager'}
  <ManagerWindow id={win.id} title={win.title} />
{:else if stage === 'window' && win && win.kind === 'settings' && session}
  <SettingsWindow {session} start={win.arg} />
{:else if stage === 'window' && win && win.kind === 'rdp'}
  <DesktopWindow id={win.id} title={win.title} nativeWindows={st?.native ?? false} />
{:else if stage === 'window' && win && win.kind === 'asst'}
  <AssistantWindow id={win.id} title={win.title} nativeWindows={st?.native ?? false} />
{:else if stage === 'window' && win && win.kind === 'editor'}
  <EditorWindow id={win.id} path={win.arg} />
{:else if stage === 'main' && st && session}
  <Shell server={st.profile?.name ?? ''} onlogout={logout} flush={page.page === 'server'} nativeWindows={st.native}>
    {#if page.page === 'server'}<ServerPage id={page.id} tab={page.tab} nativeWindows={st.native} />{:else if nav.section !== 'console'}<Dashboard nativeWindows={st.native} host={st.profile?.address.replace(/:\d+$/, '').replace(/^\[(.*)\]$/, '$1') ?? ''} />{/if}
    {#if nav.section === 'console' || con.seen}<div class="con-host" hidden={nav.section !== 'console' || page.page === 'server'}><Console nativeWindows={st.native} /></div>{/if}
  </Shell>
{/if}

<Confirm />
