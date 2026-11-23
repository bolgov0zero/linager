<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Session } from './api';
  import { locale, setLocale, t, tr, type Locale } from './i18n';
  import Dashboard from './lib/Dashboard.svelte';
  import { dashView } from './lib/dashboard/view';
  import Pult from './lib/pult/Pult.svelte';
  import Login from './lib/Login.svelte';
  import SettingsModal from './lib/settings/SettingsModal.svelte';
  import type { SettingsSection } from './lib/settings/sections';
  import { settingsRequest } from './lib/settings/request';
  import Toast from './lib/Toast.svelte';
  import { showToast } from './lib/toast';
  import AddServerModal from './lib/addserver/AddServerModal.svelte';
  import ServerPage from './lib/server/ServerPage.svelte';
  import TermWindows from './lib/terminal/TermWindows.svelte';
  import { closeAllTermWindows } from './lib/terminal/windows';
  import Modal from './lib/Modal.svelte';
  import PixIcon from './lib/ui/PixIcon.svelte';
  import TopMenu from './lib/TopMenu.svelte';
  import KeysModal from './lib/keys/KeysModal.svelte';
  import { closeVault, keysOpen, vault } from './lib/keys/store';
  import RebootButton from './lib/server/tabs/RebootButton.svelte';
  import { serverBar } from './lib/server/serverbar';
  import ViewSwitch from './lib/dashboard/ViewSwitch.svelte';
  import TabBar, { type TabBarItem } from './lib/ui/TabBar.svelte';
  import { isPhone } from './lib/ui/viewport';
  import { myAccess } from './lib/assistant/access';

  type Theme = 'light' | 'dark';
  type Phase = 'loading' | 'login' | 'app';

  const SRV_PREFIX = '#srv-';

  function initialTheme(): Theme {
    const attr = document.documentElement.dataset.theme;
    if (attr === 'light' || attr === 'dark') return attr;
    return 'dark'; // the system setting is not followed; theme-init.js has normally set the attribute already
  }

  /**
   * '#srv-<encoded name>[/<tab>]' is the server page (the name is URI-encoded, so it never contains a slash);
   * anything else is the dashboard.
   */
  function parseServerHash(value: string): { name: string; tab: string | null } | null {
    if (!value.startsWith(SRV_PREFIX)) return null;
    const rest = value.slice(SRV_PREFIX.length);
    const slash = rest.indexOf('/');
    const raw = slash < 0 ? rest : rest.slice(0, slash);
    const tab = slash < 0 ? '' : rest.slice(slash + 1);
    if (!raw) return null;
    try {
      return { name: decodeURIComponent(raw), tab: tab || null };
    } catch {
      return null;
    }
  }

  function serverHash(name: string, tab?: string): string {
    const base = `${SRV_PREFIX}${encodeURIComponent(name)}`;
    return tab && tab !== 'overview' ? `${base}/${encodeURIComponent(tab)}` : base;
  }

  let theme = $state<Theme>(initialTheme());
  let phase = $state<Phase>('loading');
  let session = $state<Session | null>(null);
  let settingsOpen = $state(false);
  let settingsSection = $state<SettingsSection>('profile');
  let settingsList = $state(false);
  let editing = $state(false);
  let addOpen = $state(false);
  let reloadKey = $state(0);
  let moreOpen = $state(false);
  let hash = $state(location.hash);
  let openedFromApp = false;

  const serverRoute = $derived(parseServerHash(hash));
  const serverName = $derived(serverRoute?.name ?? null);
  const onDashboard = $derived(phase === 'app' && serverName === null);
  // The Console ("Пульт"; not on a phone) is mounted when it is first opened and then stays mounted, so that its
  // sessions stay connected while the person looks at other views and pages.
  const pultView = $derived(onDashboard && $dashView === 'pult' && !$isPhone);
  let pultVisited = $state(false);
  $effect(() => {
    if (pultView) pultVisited = true;
  });
  let updatesCount = $state(0);
  /** Only an administrator changes the settings, adds and removes servers and agents, and edits the dashboard. */
  const isAdmin = $derived((session?.role ?? 'admin') === 'admin');

  // what the person may do, for the parts that are far from the session (the Dispatcher of the Assistant)
  $effect(() => {
    myAccess.set({ role: session?.role ?? 'auditor', modules: session?.modules ?? [] });
  });

  /** Phone: the bottom bar of the dashboard. The server page has its own bar. */
  const barItems = $derived<TabBarItem[]>([
    { id: 'servers', label: $tr('nav.servers'), icon: 'servers', current: true },
    ...(isAdmin
      ? [
          { id: 'updates', label: $tr('nav.linager'), icon: 'updates', badge: updatesCount > 0 ? updatesCount : undefined },
          { id: 'audit', label: $tr('nav.audit'), icon: 'audit' },
        ]
      : []),
    { id: 'more', label: $tr('nav.more'), icon: 'gear' },
  ]);

  function onBar(id: string): void {
    if (id === 'updates' || id === 'audit') openSettings(id);
    else if (id === 'more') moreOpen = true;
    else if (id === 'servers' && location.hash) location.hash = '';
  }

  /** The old pages #updates and #audit are sections of the settings now; links to them keep working. */
  function openLegacyHash(): boolean {
    const name = location.hash.slice(1);
    if (name !== 'updates' && name !== 'audit') return false;
    history.replaceState(history.state, '', `${location.pathname}${location.search}`);
    hash = '';
    if (session) openSettings(name);
    return true;
  }

  function openKeys(): void {
    keysOpen.set(true);
    moreOpen = false;
  }

  function openSettings(section: SettingsSection = settingsSection, list = false): void {
    settingsList = list && isAdmin;
    settingsSection = isAdmin ? section : 'profile'; // the others only have their profile
    settingsOpen = true;
    moreOpen = false;
  }

  function onHashChange(): void {
    if (openLegacyHash()) return;
    hash = location.hash;
    if (!hash.startsWith(SRV_PREFIX)) openedFromApp = false;
  }

  function openServer(name: string, tab?: string): void {
    openedFromApp = true;
    location.hash = serverHash(name, tab);
  }

  /** Keeps the active tab in the hash without adding a history entry (replaceState fires no hashchange). */
  function setServerTab(name: string, tab: string): void {
    const next = serverHash(name, tab);
    if (location.hash === next) return;
    history.replaceState(history.state, '', `${location.pathname}${location.search}${next}`);
    hash = next;
  }

  function backToDashboard(): void {
    if (openedFromApp) history.back();
    else location.hash = '';
  }

  async function refreshUpdatesCount(): Promise<void> {
    if (!isAdmin) return;
    try {
      updatesCount = (await api.updatesSummary()).count;
    } catch {
      // not signed in yet or offline: keep the previous number
    }
  }

  function toggleTheme(): void {
    theme = theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = theme;
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme === 'dark' ? '#0f1218' : '#e8ebe5');
    try {
      localStorage.setItem('linager.theme', theme); // the choice stays after a reload and does not depend on the system
    } catch {
      // storage is blocked (private mode): the choice lasts until the page is reloaded
    }
  }

  function pickLocale(next: Locale): void {
    setLocale(next);
  }

  function toggleEdit(): void {
    editing = !editing;
    if (editing) showToast(t('dash.editHint'));
  }

  function startSession(next: Session): void {
    session = next;
    phase = 'app';
    if (next.totp_required) openSettings('profile'); // 2FA is required: the profile has its setup
    void refreshUpdatesCount();
  }

  function endSession(): void {
    session = null;
    settingsOpen = false;
    keysOpen.set(false);
    closeVault(); // the decrypted keys are forgotten with the sign-in
    editing = false;
    addOpen = false;
    pultVisited = false; // the sessions of the Console end with the sign-in
    closeAllTermWindows();
    if (hash) {
      history.replaceState(null, '', `${location.pathname}${location.search}`);
      hash = '';
    }
    phase = 'login';
  }

  async function logout(): Promise<void> {
    try {
      await api.logout();
    } catch {
      // Drop the local session even if the server call failed.
    }
    endSession();
  }

  /** Reloads the session data after the profile changed it (2FA, passkeys, recovery codes). */
  async function refreshSession(): Promise<void> {
    try {
      session = await api.me();
    } catch {
      endSession();
    }
  }

  async function restoreSession(): Promise<void> {
    try {
      startSession(await api.me());
    } catch {
      endSession();
    }
  }

  // other components ask for a settings section ("Manage" in the snippets list of the terminal)
  let handledRequest = 0;
  $effect(() => {
    const req = $settingsRequest;
    if (req && req.seq !== handledRequest && session) {
      handledRequest = req.seq;
      openSettings(req.section);
    }
  });

  onMount(() => {
    window.addEventListener('hashchange', onHashChange);
    void restoreSession().then(() => openLegacyHash());
    const counter = setInterval(() => session && void refreshUpdatesCount(), 60_000);
    return () => {
      clearInterval(counter);
      window.removeEventListener('hashchange', onHashChange);
    };
  });
</script>

<header class="top" class:hidden-phone={serverName !== null} class:guest={phase !== 'app'}>
  <div class="brand">
    <svg class="logo" viewBox="0 0 14 14" shape-rendering="crispEdges" aria-hidden="true">
      <rect width="14" height="14" rx="3" fill="#0f1218" />
      <rect x="2" y="3" width="2" height="2" fill="#e6e9ef" />
      <rect x="4" y="5" width="2" height="2" fill="#e6e9ef" />
      <rect x="6" y="7" width="2" height="2" fill="#e6e9ef" />
      <rect x="4" y="9" width="2" height="2" fill="#e6e9ef" />
      <rect x="8" y="10" width="4" height="2" fill="#7d9cff" />
    </svg>
    <h1>{$tr('app.title')}</h1>
  </div>
  {#if serverName !== null && $serverBar}
    {@const sb = $serverBar}
    <div class="here">
      <button class="btn" type="button" onclick={sb.onback}>{$tr('srv.back')}</button>
      <span class="sdot" class:online={sb.online} aria-hidden="true"></span>
      <h2 class="sname" title={sb.detail.name}>{sb.detail.name}</h2>
      <span class="sstate" class:online={sb.online} class:hint={sb.lastSeen !== ''} title={sb.lastSeen || undefined}>{sb.online ? $tr('srv.online') : $tr('srv.offline')}</span>
    </div>
  {/if}
  <div class="right">
    {#if onDashboard}
      <ViewSwitch />
      {#if editing}
        <button class="btn ink" type="button" onclick={toggleEdit}>{$tr('dash.done')}</button>
      {/if}
      {#if isAdmin}
        <button class="btn primary add-text" type="button" onclick={() => (addOpen = true)}>{$tr('servers.add')}</button>
        <button class="btn primary phone-add" type="button" aria-label={$tr('servers.add')} onclick={() => (addOpen = true)}>
          <PixIcon name="plus" />
        </button>
      {/if}
    {/if}
    {#if serverName !== null && $serverBar}
      {@const sb = $serverBar}
      <button class="btn tb-ic note" class:has={sb.hasNote} type="button" title={$tr('srv.note.open')} aria-label={$tr('srv.note.open')} onclick={sb.onnote}><PixIcon name="note" size={16} /></button>
      {#if sb.isAdmin}
        <RebootButton server={sb.detail} icon />
        <button class="btn tb-ic danger" type="button" title={$tr('srv.delete')} aria-label={$tr('srv.delete')} onclick={sb.ondelete}><PixIcon name="trash" size={16} /></button>
      {/if}
    {/if}
    <!-- Phone: the bottom bar has its own "More" sheet, so the menu is only for the sign-in screen there. -->
    {#if !($isPhone && phase === 'app')}
      <TopMenu
        dark={theme === 'dark'}
        signedIn={phase === 'app'}
        {isAdmin}
        canEdit={onDashboard && isAdmin}
        {editing}
        {updatesCount}
        ontheme={toggleTheme}
        onlocale={pickLocale}
        onedit={toggleEdit}
        onsettings={() => openSettings()}
        onkeys={openKeys}
        onlogout={() => void logout()} />
    {/if}
  </div>
</header>

<main class:center={phase === 'login'} class:bar={phase === 'app'} class:srvpage={serverName !== null} class:pultmain={pultView}>
  {#if phase === 'loading'}
    <p class="muted">{$tr('common.loading')}</p>
  {:else if phase === 'login'}
    <Login onlogin={startSession} />
  {:else if session}
    {#if serverName !== null}
      {#key serverName}
        <ServerPage
          name={serverName}
          tab={serverRoute?.tab ?? null}
          ontab={(next) => setServerTab(serverName, next)}
          onBack={backToDashboard}
          role={session.role ?? 'admin'}
          userModules={session.modules ?? []}
          onagentchanged={refreshUpdatesCount} />
      {/key}
    {:else if !pultView}
      <Dashboard {editing} {reloadKey} onopen={openServer} onauthlost={endSession} />
    {/if}
  {/if}
</main>

{#if addOpen && session}
  <AddServerModal
    onclose={() => (addOpen = false)}
    onadded={() => (reloadKey += 1)}
    onopen={openServer}
    onauthlost={endSession}
  />
{/if}

{#if settingsOpen && session}
  <SettingsModal
    section={settingsSection}
    list={settingsList}
    {session}
    {updatesCount}
    onsection={(next) => (settingsSection = next)}
    onclose={() => (settingsOpen = false)}
    onauthlost={endSession}
    onrefresh={refreshSession}
    onupdateschanged={refreshUpdatesCount}
  />
{/if}

{#if $keysOpen && session}
  <KeysModal onclose={() => keysOpen.set(false)} />
{/if}

{#if session && $isPhone && serverName === null}
  <TabBar items={barItems} label={$tr('app.menu')} onselect={onBar} />
{/if}

{#if moreOpen && session}
  <Modal title={$tr('more.title')} labelId="more-title" onclose={() => (moreOpen = false)} flush>
    <div class="more">
      <button class="act" type="button" onclick={() => openSettings(settingsSection, true)}>
        <PixIcon name="gear" />{isAdmin ? $tr('app.settings') : $tr('app.profile')}
        <PixIcon name="next" />
      </button>
      <button class="act" type="button" onclick={openKeys}>
        <PixIcon name={$vault ? 'unlock' : 'key'} />{$tr('keys.menu')}
        <PixIcon name="next" />
      </button>
      {#if isAdmin}
        <button class="act" type="button" onclick={() => { moreOpen = false; toggleEdit(); }}>
          <PixIcon name="cards" />{editing ? $tr('dash.done') : $tr('dash.edit')}
          <PixIcon name="next" />
        </button>
      {/if}
      <div class="act plain">
        <span class="lab">{$tr('more.theme')}</span>
        <div class="seg" role="group" aria-label={$tr('more.theme')}>
          <button class="chip" type="button" aria-pressed={theme === 'light'} onclick={() => theme !== 'light' && toggleTheme()}>{$tr('theme.light')}</button>
          <button class="chip" type="button" aria-pressed={theme === 'dark'} onclick={() => theme !== 'dark' && toggleTheme()}>{$tr('theme.dark')}</button>
        </div>
      </div>
      <div class="act plain">
        <span class="lab">{$tr('more.language')}</span>
        <div class="seg" role="group" aria-label="language">
          <button class="chip" type="button" aria-pressed={$locale === 'ru'} onclick={() => pickLocale('ru')}>{$tr('lang.ru')}</button>
          <button class="chip" type="button" aria-pressed={$locale === 'en'} onclick={() => pickLocale('en')}>{$tr('lang.en')}</button>
        </div>
      </div>
      <div class="act plain">
        <button class="btn danger logout" type="button" onclick={() => { moreOpen = false; void logout(); }}>{$tr('app.logout')}</button>
      </div>
    </div>
  </Modal>
{/if}

{#if session && pultVisited}
  <Pult shown={pultView} role={session.role ?? 'admin'} modules={session.modules ?? []} {reloadKey} onopen={openServer} onauthlost={endSession} />
{/if}

{#if session}
  <TermWindows canUsers={(session.role ?? 'admin') === 'admin' || (session.role === 'operator' && (session.modules ?? []).includes('users'))} />
{/if}

<Toast />

<style>
  .top {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 9px;
    padding: 7px 14px;
    background: var(--surface);
    border-bottom: 2px solid var(--line);
    box-shadow: 3px 3px 0 var(--shade);
  }

  /* The top bar stays pinned while the page scrolls (below floating terminals and dialogs). */
  .top {
    position: sticky;
    top: 0;
    z-index: 10;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 11px;
  }

  .logo {
    width: 28px;
    height: 28px;
    display: block;
  }

  .brand h1 {
    margin: 0;
    font-variation-settings: "ELSH" 40;
    font-size: 23px;
    letter-spacing: 0.02em;
    line-height: 1;
  }

  /* The server page: back, the name and the state stand next to the logo, the actions are icons on the right. */
  .here {
    display: flex;
    align-items: center;
    gap: 9px;
    flex: 1 1 0;
    min-width: 0;
    margin-left: 4px;
  }

  .here .btn {
    flex: none;
    white-space: nowrap;
  }

  .sdot {
    flex: none;
    width: 10px;
    height: 10px;
    background: var(--muted);
  }

  .sdot.online {
    background: var(--good);
  }

  .sname {
    margin: 0;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-variation-settings: "ELSH" 40;
    font-size: 22px;
    line-height: 1.1;
  }

  .sstate {
    flex: none;
    padding: 1px 7px;
    border: 2px solid var(--muted);
    color: var(--muted);
    font-size: 11px;
    line-height: 1.4;
  }

  .sstate.online {
    border-color: var(--good);
    color: var(--good);
  }

  /* An offline server tells when it was last seen in a tooltip. */
  .sstate.hint {
    cursor: help;
  }

  /* The icon buttons of the server page; the note button has a small square when the server has a note. */
  .right :global(.tb-ic) {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 29px;
    height: 29px;
    padding: 0;
  }

  .right :global(.tb-ic.has)::after {
    content: '';
    position: absolute;
    top: 3px;
    right: 3px;
    width: 5px;
    height: 5px;
    background: var(--accent);
  }

  .right {
    display: flex;
    flex-wrap: wrap;
    align-items: stretch;
    gap: 7px;
  }

  /* :where() keeps this rule weaker than .add-text and .phone-add below. */
  .right :global(:where(.btn, .chip)) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .phone-add {
    display: none;
  }

  .seg {
    display: flex;
    gap: 5px;
  }

  /* Phones: a compact bar (logo, live indicator, view and add buttons) that stays pinned; the rest is in the bottom bar. */
  @media (max-width: 720px) {
    .top {
      flex-wrap: nowrap;
      padding: 7px 11px;
    }

    .logo {
      width: 28px;
      height: 28px;
    }

    .brand {
      gap: 7px;
    }

    .brand h1 {
      font-size: 20px;
    }

    .right {
      flex-wrap: nowrap;
      align-items: center;
      gap: 7px;
    }

    .add-text {
      display: none;
    }

    .phone-add {
      display: inline-flex;
      width: 44px;
      min-height: 44px;
      padding: 0;
    }

    .top {
      padding-top: calc(5px + var(--safe-t));
    }

    .top.hidden-phone {
      display: none;
    }

  }

  main {
    padding: 14px 14px;
  }

  @media (max-width: 720px) {
    main {
      padding: 11px;
    }

    main.bar {
      padding-bottom: calc(var(--bar-h) + var(--safe-b) + 14px);
    }

    main.srvpage {
      padding: 0;
    }
  }

  /* The sign-in card sits in the middle of the free space under the top bar. */
  /* the Console is fixed under the top bar and fills the window by itself */
  main.pultmain {
    display: none;
  }

  main.center {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: calc(100dvh - 56px); /* the top bar is 62 px high (50 px on phones) */
  }

  @media (max-width: 720px) {
    main.center {
      min-height: calc(100dvh - 52px - var(--safe-t));
    }
  }
  .more .act {
    display: flex;
    align-items: center;
    gap: 11px;
    width: 100%;
    min-height: 54px;
    padding: 0 13px;
    background: none;
    border: 0;
    border-top: 2px dashed var(--seg-off);
    color: var(--ink);
    font-family: var(--font-display);
    font-size: 14px;
    text-align: left;
  }

  .more .act:first-child {
    border-top: 0;
  }

  .more button.act > :global(svg:last-child) {
    margin-left: auto;
  }

  .more .act.plain {
    justify-content: space-between;
  }

  .more .act .lab {
    color: var(--muted);
  }

  .more .logout {
    width: 100%;
  }
</style>
