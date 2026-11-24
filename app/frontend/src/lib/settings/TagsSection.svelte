<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type TagRow } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import { isUnauthorized } from '../errors';
  import { t } from '../i18n.svelte';
  import { TAG_COLORS } from '../tag-colors';

  // The tag registry: the tags that were typed in the settings of servers and the ones made here. A tag can be renamed and
  // recoloured (servers and grants keep it), and deleted (it goes off every server and out of every grant).
  let { onauthlost }: { onauthlost: () => void } = $props();

  const MAX_CHARS = 32;
  let tags = $state<TagRow[]>([]);
  let max = $state(200);
  let loaded = $state(false);
  let newName = $state('');
  let newColor = $state<string>('gray');
  let names = $state<Record<number, string>>({});
  let picking = $state<number | null>(null);
  let error = $state('');
  let busy = $state(false);

  function fail(e: unknown): void {
    if (isUnauthorized(e)) return onauthlost();
    if (e instanceof ApiError && e.status === 409) error = t('st.tags.exists');
    else if (e instanceof ApiError && e.status === 400) error = t('st.tags.invalid', { n: MAX_CHARS });
    else if (e instanceof ApiError && e.status === 422) error = t('st.tags.full', { n: max });
    else error = t('st.tags.failed');
  }
  async function load(): Promise<void> {
    try {
      const r = await api.tags();
      tags = r.tags;
      max = r.max;
      names = {};
    } catch (e) {
      fail(e);
    } finally {
      loaded = true;
    }
  }
  /** Runs a change, then reloads the list (the order and the counts come from the server). */
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
  const clean = (v: string): string => v.split(/\s+/).filter(Boolean).join(' ');
  async function add(): Promise<void> {
    const name = clean(newName);
    if (name && (await run(() => api.tagCreate(name, newColor)))) newName = '';
  }
  async function rename(tag: TagRow): Promise<void> {
    const name = clean(names[tag.id] ?? tag.name);
    if (name && name !== tag.name) await run(() => api.tagUpdate(tag.id, { name }));
  }
  async function recolor(tag: TagRow, color: string): Promise<void> {
    picking = null;
    if (color !== tag.color) await run(() => api.tagUpdate(tag.id, { color }));
  }
  async function remove(tag: TagRow): Promise<void> {
    const ok = await confirmAsk({ title: t('st.tags.delete'), text: t('st.tags.deleteText', { name: tag.name, servers: tag.servers, users: tag.users }), ok: t('st.tags.delete'), danger: true });
    if (ok) await run(() => api.tagDelete(tag.id));
  }
  onMount(() => void load());
</script>

<div class="set-sec">
  <h3>{t('st.tags.title')}</h3>
  <p class="dim">{t('st.tags.hint')}</p>
  <form class="mdl-stack" onsubmit={(e) => { e.preventDefault(); void add(); }}>
    <div class="row">
      <span class="field grow"><input maxlength={MAX_CHARS} autocomplete="off" spellcheck="false" placeholder={t('st.tags.new')} aria-label={t('st.tags.new')} bind:value={newName} /></span>
      <button class="btn primary" type="submit" disabled={busy || !clean(newName) || tags.length >= max}>{t('st.tags.add')}</button>
    </div>
    <div class="tpick" role="group" aria-label={t('st.tags.color')}>
      {#each TAG_COLORS as c (c)}
        <button class="tsw tc-{c}" type="button" title={t(`st.tags.c.${c}`)} aria-label={t(`st.tags.c.${c}`)} aria-pressed={newColor === c} onclick={() => (newColor = c)}></button>
      {/each}
    </div>
  </form>
  {#if error}<div class="err" role="alert">{error}</div>{/if}
  {#if !loaded}<p class="dim">{t('dash.loading')}</p>
  {:else if tags.length === 0}<p class="dim">{t('st.tags.empty')}</p>
  {:else}
    <ul class="set-list">
      {#each tags as tag (tag.id)}
        <li class="tag-li">
          <div class="row">
            <button class="tsw tc-{tag.color}" type="button" title={t('st.tags.color')} aria-label={t('st.tags.color')} aria-expanded={picking === tag.id} disabled={busy} onclick={() => (picking = picking === tag.id ? null : tag.id)}></button>
            <span class="field grow"><input maxlength={MAX_CHARS} autocomplete="off" spellcheck="false" aria-label={t('st.tags.rename')} value={names[tag.id] ?? tag.name} oninput={(e) => (names[tag.id] = e.currentTarget.value)} onkeydown={(e) => e.key === 'Enter' && void rename(tag)} /></span>
            <span class="dim nowrap">{t('st.tags.servers', { n: tag.servers })} · {t('st.tags.users', { n: tag.users })}</span>
            <button class="btn sm" type="button" disabled={busy || !clean(names[tag.id] ?? tag.name) || clean(names[tag.id] ?? tag.name) === tag.name} onclick={() => void rename(tag)}>{t('st.tags.save')}</button>
            <button class="btn sm danger" type="button" disabled={busy} onclick={() => void remove(tag)}>{t('st.tags.delete')}</button>
          </div>
          {#if picking === tag.id}
            <div class="tpick" role="group" aria-label={t('st.tags.color')}>
              {#each TAG_COLORS as c (c)}
                <button class="tsw tc-{c}" type="button" title={t(`st.tags.c.${c}`)} aria-label={t(`st.tags.c.${c}`)} aria-pressed={tag.color === c} onclick={() => void recolor(tag, c)}></button>
              {/each}
            </div>
          {/if}
        </li>
      {/each}
    </ul>
    <p class="dim">{t('st.tags.count', { n: tags.length, max })}</p>
  {/if}
</div>
