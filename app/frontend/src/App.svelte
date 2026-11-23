<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type Session } from './lib/api';
  import Connect from './lib/Connect.svelte';
  import Dashboard from './lib/Dashboard.svelte';
  import { t } from './lib/i18n.svelte';
  import Login from './lib/Login.svelte';
  import { native, type AppState } from './lib/native';
  import Shell from './lib/Shell.svelte';
  import Terminal from './lib/Terminal.svelte';

  // A window of the application is a hash route: #/w/terminal/<server id>?title=<name>. Everything else is the main window.
  function route(): { kind: string; id: string; title: string } | null {
    const m = /^#\/w\/(\w+)\/([^?]+)(?:\?(.*))?$/.exec(location.hash);
    if (!m) return null;
    return { kind: m[1], id: decodeURIComponent(m[2]), title: new URLSearchParams(m[3] ?? '').get('title') ?? m[2] };
  }
  const win = route();

  type Stage = 'loading' | 'connect' | 'login' | 'main' | 'window' | 'error';
  let stage = $state<Stage>('loading');
  let st = $state<AppState | null>(null);
  let session = $state<Session | null>(null);

  async function enter(s: AppState): Promise<void> {
    st = s;
    if (!s.connected) {
      stage = 'connect';
      return;
    }
    try {
      session = await api.me(); // the cookie of an earlier sign-in may still be alive
      stage = win ? 'window' : 'main';
    } catch (e) {
      if (win && e instanceof ApiError && e.status === 401) {
        stage = 'error';
        return;
      }
      stage = 'login';
    }
  }

  onMount(async () => {
    try {
      await enter(await native.state());
    } catch {
      stage = 'error';
    }
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
  <Login profile={st.profile} onsignedin={(s) => { session = s; stage = 'main'; }} onchange={(s) => void enter(s)} />
{:else if stage === 'window' && win && win.kind === 'terminal'}
  <Terminal id={win.id} title={win.title} />
{:else if stage === 'main' && st && session}
  <Shell server={st.profile?.name ?? ''} onlogout={logout}><Dashboard nativeWindows={st.native} /></Shell>
{/if}
