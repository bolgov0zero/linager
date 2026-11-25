<script lang="ts">
  import { onMount } from 'svelte';
  import { chainOf, pathOf } from '../folders';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import { portal } from '../portal';
  import KeyOpenForm from './KeyOpenForm.svelte';
  import { checkVaultKnown, keyState, loadDirectory, touch, vault } from './store';
  import { accountOf, sectionsFor, type Entry, type Known, type Sections, type Target } from './vault';

  // The list "Key" of a login form of a server: the keys of the open vault that fit it, this server's first. Picking one
  // hands it to the form. In the icon variant (a toolbar) each key has the buttons of what to type.
  let { serverId, use, variant = 'field', picked = null, onpick, onact, title = '' }: {
    serverId: string; use: 'rdp' | 'uac'; variant?: 'field' | 'icon'; picked?: string | null;
    onpick?: (entry: Entry) => void; onact?: (entry: Entry, action: 'login' | 'password' | 'signin') => void; title?: string;
  } = $props();

  const keyTitle = (base: string, state: string): string => (state === 'none' ? base : `${base} (${t('keys.state.' + state)})`);
  onMount(() => void checkVaultKnown());

  let button = $state<HTMLButtonElement | null>(null);
  let open = $state(false);
  let more = $state(false);
  let pos = $state({ left: 0, top: 0, width: 0, up: false, max: 280 });
  let target = $state<Target>({ serverId: '', groupId: null });
  let known = $state<Known>({ servers: new Set(), groups: new Set() });
  let serverName = $state('');
  let groupName = $state('');
  let list = $state<HTMLDivElement | null>(null);

  function place(): void {
    if (!button) return;
    const r = button.getBoundingClientRect();
    const below = window.innerHeight - r.bottom - 8;
    const above = r.top - 8;
    const up = below < 180 && above > below;
    // the unlock field is small (320 px); the list of the keys needs a little more
    const width = variant === 'icon' ? Math.min($vault ? 380 : 320, window.innerWidth - 16) : r.width;
    const left = variant === 'icon' ? Math.max(8, Math.min(r.left, window.innerWidth - width - 8)) : r.left;
    pos = { left, top: up ? r.top - 4 : r.bottom + 4, width, up, max: Math.max(120, Math.min(300, up ? above : below)) };
  }
  async function show(): Promise<void> {
    place();
    open = true;
    more = false;
    touch();
    try {
      const dir = await loadDirectory();
      const here = dir.servers.find((s) => s.id === serverId);
      target = { serverId, groupId: here?.group_id ?? null, groupChain: chainOf(dir.groups, here?.group_id ?? null) };
      serverName = here?.name ?? '';
      groupName = pathOf(dir.groups, target.groupId);
      known = { servers: new Set(dir.servers.map((s) => s.id)), groups: new Set(dir.groups.map((g) => String(g.id))) };
    } catch {
      // the order is then by what is known
    }
  }
  const hide = (): void => void (open = false);
  const sections = $derived<Sections>($vault ? sectionsFor($vault.entries, target, known, use) : { here: [], group: [], all: [], other: [] });
  const fitting = $derived(sections.here.length + sections.group.length + sections.all.length);

  function pick(e: Entry): void {
    hide();
    touch();
    onpick?.(e);
  }
  function act(e: Entry, action: 'login' | 'password' | 'signin'): void {
    hide();
    touch();
    onact?.(e, action);
  }
  function outside(ev: PointerEvent): void {
    if (open && !button?.contains(ev.target as Node) && !list?.contains(ev.target as Node)) hide();
  }
</script>

<svelte:window onpointerdown={outside} onresize={hide} onkeydown={(e) => open && e.key === 'Escape' && (e.stopPropagation(), hide())} />

{#snippet item(e: Entry)}
  {#if variant === 'icon'}
    <div class="key" role="option" aria-selected="false">
      <span class="nm" title={accountOf(e)}>{e.name}</span>
      <span class="btns">
        <button class="btn sm" type="button" title={t('keys.act.loginHint', { account: accountOf(e) })} onclick={() => act(e, 'login')}>{t('keys.act.login')}</button>
        <button class="btn sm" type="button" disabled={!e.password} title={t('keys.act.passwordHint')} onclick={() => act(e, 'password')}>{t('keys.act.password')}</button>
        <button class="btn sm primary" type="button" disabled={!e.password} title={t('keys.act.signinHint')} onclick={() => act(e, 'signin')}>{t('keys.act.signin')}</button>
      </span>
    </div>
  {:else}
    <button class="key" type="button" role="option" aria-selected="false" onclick={() => pick(e)}><span class="nm">{e.name}</span><small>{accountOf(e)}</small></button>
  {/if}
{/snippet}

{#if variant === 'icon'}
  <button bind:this={button} class="btn sm icon" type="button" data-keys={$keyState} aria-haspopup="listbox" aria-expanded={open} title={keyTitle(title, $keyState)} aria-label={keyTitle(title, $keyState)} onclick={() => (open ? hide() : void show())}><Icon name={$vault ? 'lock-open' : 'key-round'} size={14} /></button>
{:else}
  <div class="fl"><span>{t('keys.pick.label')}</span>
    <button bind:this={button} class="btn" type="button" style="justify-content:space-between;width:100%" aria-haspopup="listbox" aria-expanded={open} onclick={() => (open ? hide() : void show())}>
      <span>{#if picked}<Icon name="key-round" size={14} /> {picked}{:else}<span class="dim">{t('keys.pick.ph')}</span>{/if}</span><Icon name="chevron-down" size={13} />
    </button></div>
{/if}

{#if open}
  <div bind:this={list} class="pop keylist" class:kc-pop={!$vault} use:portal={{ near: button }} role={$vault ? 'listbox' : 'dialog'} aria-label={t('keys.pick.label')} style="left:{pos.left}px;top:{pos.up ? 'auto' : `${pos.top}px`};bottom:{pos.up ? `${window.innerHeight - pos.top}px` : 'auto'};width:{pos.width}px;max-height:{pos.max}px">
    {#if !$vault}
      <!-- the vault is locked: its unlocking is here, under the button, not in a window of its own -->
      <div class="keyunlock"><KeyOpenForm compact onopened={() => { touch(); void show(); }} /></div>
    {:else}
      {#if sections.here.length}<div class="gh">{serverName ? t('keys.pick.here', { name: serverName }) : t('keys.pick.thisServer')}</div>{#each sections.here as e (e.id)}{@render item(e)}{/each}{/if}
      {#if sections.group.length}<div class="gh">{t('keys.pick.group', { name: groupName })}</div>{#each sections.group as e (e.id)}{@render item(e)}{/each}{/if}
      {#if sections.all.length}<div class="gh">{t('keys.pick.all')}</div>{#each sections.all as e (e.id)}{@render item(e)}{/each}{/if}
      {#if fitting === 0}<div class="key" style="cursor:default"><span class="dim">{t('keys.pick.none')}</span></div>{/if}
      {#if sections.other.length}
        <button class="key" type="button" onclick={() => (more = !more)}><span class="dim">{more ? t('keys.pick.hideAll') : t('keys.pick.showAll', { n: sections.other.length })}</span></button>
        {#if more}<div class="gh">{t('keys.pick.other')}</div>{#each sections.other as e (e.id)}{@render item(e)}{/each}{/if}
      {/if}
    {/if}
  </div>
{/if}
