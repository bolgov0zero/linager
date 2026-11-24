<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type Snippet } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import { isUnauthorized } from '../errors';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import { showToast } from '../toast';

  let { onauthlost }: { onauthlost: () => void } = $props();

  let snippets = $state<Snippet[]>([]);
  let loaded = $state(false);
  let editId = $state<number | 'new' | null>(null);
  let name = $state('');
  let command = $state('');
  let error = $state('');
  let busy = $state(false);

  async function load(): Promise<void> {
    try {
      snippets = (await api.snippetsList()).snippets;
    } catch (e) {
      fail(e);
    } finally {
      loaded = true;
    }
  }
  onMount(() => void load());

  function edit(sn: Snippet | null): void {
    editId = sn ? sn.id : 'new';
    name = sn?.name ?? '';
    command = sn?.command ?? '';
    error = '';
  }
  function fail(e: unknown): void {
    if (isUnauthorized(e)) return onauthlost();
    error = e instanceof ApiError && e.status === 422 && e.code ? e.code : t('common.genericError');
  }
  async function save(): Promise<void> {
    if (busy || editId === null) return;
    busy = true;
    error = '';
    try {
      if (editId === 'new') await api.snippetCreate(name.trim(), command);
      else await api.snippetUpdate(editId, name.trim(), command);
      editId = null;
      showToast(t('st.snippets.saved'));
      await load();
    } catch (e) {
      fail(e);
    } finally {
      busy = false;
    }
  }
  async function remove(sn: Snippet): Promise<void> {
    if (!(await confirmAsk({ title: t('st.snippets.delete'), text: t('st.snippets.confirmDelete', { name: sn.name }), ok: t('st.snippets.delete'), danger: true }))) return;
    busy = true;
    try {
      await api.snippetDelete(sn.id);
      showToast(t('st.snippets.deleted'));
      await load();
    } catch (e) {
      fail(e);
    } finally {
      busy = false;
    }
  }
  async function move(index: number, delta: number): Promise<void> {
    const list = [...snippets];
    const j = index + delta;
    if (j < 0 || j >= list.length) return;
    [list[index], list[j]] = [list[j], list[index]];
    snippets = list.map((s, i) => ({ ...s, position: i }));
    try {
      await api.snippetsOrder(list.map((s) => s.id));
    } catch (e) {
      fail(e);
      await load();
    }
  }
</script>

<div class="set-sec">
  <div class="row"><h3 style="margin:0">{t('st.snippets.title')}</h3><span class="grow"></span><button class="btn primary" type="button" onclick={() => edit(null)}>{t('st.snippets.add')}</button></div>
  <p class="dim">{t('st.snippets.hint')}</p>
  {#if editId !== null}
    <form class="panel mdl-stack" onsubmit={(e) => { e.preventDefault(); void save(); }}>
      <label class="fl"><span>{t('st.snippets.name')}</span><span class="field"><input maxlength="80" required bind:value={name} /></span></label>
      <label class="fl"><span>{t('st.snippets.command')}</span><textarea class="mono area" rows="4" required spellcheck="false" bind:value={command}></textarea></label>
      {#if error}<div class="err" role="alert">{error}</div>{/if}
      <div class="row end"><button class="btn" type="button" onclick={() => (editId = null)}>{t('common.cancel')}</button><button class="btn primary" type="submit" disabled={busy}>{t('st.snippets.save')}</button></div>
    </form>
  {:else if error}<div class="err" role="alert">{error}</div>{/if}
  {#if !loaded}<p class="dim">{t('dash.loading')}</p>
  {:else if snippets.length === 0 && editId === null}<p class="dim">{t('st.snippets.empty')}</p>
  {:else}
    <ul class="set-list">
      {#each snippets as sn, i (sn.id)}
        <li>
          <div class="grow snip"><b>{sn.name}</b><code class="mono dim">{sn.command}</code></div>
          <button class="btn icon sm" type="button" aria-label={t('st.snippets.up')} title={t('st.snippets.up')} disabled={i === 0} onclick={() => void move(i, -1)}><Icon name="arrow-up" size={14} /></button>
          <button class="btn icon sm" type="button" aria-label={t('st.snippets.down')} title={t('st.snippets.down')} disabled={i === snippets.length - 1} onclick={() => void move(i, 1)}><Icon name="arrow-down" size={14} /></button>
          <button class="btn sm" type="button" onclick={() => edit(sn)}>{t('st.snippets.edit')}</button>
          <button class="btn sm danger" type="button" disabled={busy} onclick={() => void remove(sn)}>{t('st.snippets.delete')}</button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
