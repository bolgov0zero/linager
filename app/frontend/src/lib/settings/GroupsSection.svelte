<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type Group } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import { isUnauthorized } from '../errors';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';

  let { onauthlost }: { onauthlost: () => void } = $props();

  let groups = $state<Group[]>([]);
  let loaded = $state(false);
  let newName = $state('');
  let names = $state<Record<number, string>>({});
  let error = $state('');
  let busy = $state(false);

  function fail(e: unknown): void {
    if (isUnauthorized(e)) return onauthlost();
    if (e instanceof ApiError && e.status === 409) error = t('st.dash.group.exists');
    else if (e instanceof ApiError && e.status === 400) error = t('st.dash.group.invalid');
    else error = t('st.dash.group.failed');
  }
  async function load(): Promise<void> {
    try {
      groups = await api.groups();
      names = {};
    } catch (e) {
      fail(e);
    } finally {
      loaded = true;
    }
  }
  /** Runs a change, then reloads the list (the order and the names come from the server). */
  async function run(action: () => Promise<unknown>): Promise<boolean> {
    if (busy) return false;
    busy = true;
    error = '';
    try {
      await action();
      await load();
      return true;
    } catch (e) {
      fail(e);
      return false;
    } finally {
      busy = false;
    }
  }
  async function add(): Promise<void> {
    const name = newName.trim();
    if (name && (await run(() => api.groupCreate(name)))) newName = '';
  }
  async function rename(g: Group): Promise<void> {
    const name = (names[g.id] ?? g.name).trim();
    if (name && name !== g.name) await run(() => api.groupRename(g.id, name));
  }
  async function remove(g: Group): Promise<void> {
    if (!(await confirmAsk({ title: t('st.dash.group.delete'), text: g.name, ok: t('st.dash.group.delete'), danger: true }))) return;
    await run(() => api.groupDelete(g.id));
  }
  async function move(index: number, dir: -1 | 1): Promise<void> {
    const ids = groups.map((g) => g.id);
    const to = index + dir;
    if (to < 0 || to >= ids.length) return;
    [ids[index], ids[to]] = [ids[to], ids[index]];
    await run(() => api.groupsReorder(ids));
  }
  onMount(() => void load());
</script>

<div class="set-sec">
  <h3>{t('st.dash.group.title')}</h3>
  <p class="dim">{t('st.dash.group.hint')}</p>
  <form class="row" onsubmit={(e) => { e.preventDefault(); void add(); }}>
    <span class="field grow"><input maxlength="40" placeholder={t('st.dash.group.new')} aria-label={t('st.dash.group.new')} bind:value={newName} /></span>
    <button class="btn primary" type="submit" disabled={busy || !newName.trim()}>{t('st.dash.group.add')}</button>
  </form>
  {#if error}<div class="err" role="alert">{error}</div>{/if}
  {#if !loaded}<p class="dim">{t('dash.loading')}</p>
  {:else if groups.length === 0}<p class="dim">{t('st.dash.group.empty')}</p>
  {:else}
    <ul class="set-list">
      {#each groups as g, i (g.id)}
        <li>
          <span class="field grow"><input maxlength="40" aria-label={t('st.dash.group.rename')} value={names[g.id] ?? g.name} oninput={(e) => (names[g.id] = e.currentTarget.value)} onkeydown={(e) => e.key === 'Enter' && void rename(g)} /></span>
          <button class="btn icon sm" type="button" title={t('st.dash.group.up')} aria-label={t('st.dash.group.up')} disabled={busy || i === 0} onclick={() => void move(i, -1)}><Icon name="arrow-up" size={14} /></button>
          <button class="btn icon sm" type="button" title={t('st.dash.group.down')} aria-label={t('st.dash.group.down')} disabled={busy || i === groups.length - 1} onclick={() => void move(i, 1)}><Icon name="arrow-down" size={14} /></button>
          <button class="btn sm" type="button" disabled={busy || (names[g.id] ?? g.name).trim() === g.name} onclick={() => void rename(g)}>{t('st.dash.group.save')}</button>
          <button class="btn sm danger" type="button" disabled={busy} onclick={() => void remove(g)}>{t('st.dash.group.delete')}</button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
