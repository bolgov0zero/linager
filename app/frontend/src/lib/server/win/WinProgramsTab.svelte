<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError, winCall, type ServerDetail, type WinApp } from '../../api';
  import Icon from '../../Icon.svelte';
  import { t } from '../../i18n.svelte';
  import SearchField from '../../SearchField.svelte';
  import { fmtBytes } from '../../status';

  let { s }: { s: ServerDetail } = $props();
  const sid = $derived(s.id);
  type Key = 'name' | 'version' | 'publisher' | 'installed' | 'size';
  let apps = $state<WinApp[] | null>(null);
  let loading = $state(false);
  let error = $state('');
  let q = $state('');
  let sort = $state<Key>('name');
  let desc = $state(false);

  async function load(): Promise<void> {
    loading = true;
    try {
      apps = (await winCall<{ apps: WinApp[] }>(sid, 'apps', 'list')).apps;
      error = '';
    } catch (e) {
      // 404 / 501: an agent that predates the module
      error = e instanceof ApiError && (e.status === 404 || e.status === 501) ? t('wp.old') : e instanceof ApiError && e.code ? e.code : t('win.error');
    } finally {
      loading = false;
    }
  }
  onMount(() => void load());

  const rows = $derived.by(() => {
    const needle = q.trim().toLowerCase();
    const list = (apps ?? []).filter((a) => !needle || `${a.name} ${a.publisher}`.toLowerCase().includes(needle));
    const dir = desc ? -1 : 1;
    return list.sort((a, b) => {
      // an empty value (no date, no size) goes to the end whichever the direction is
      if (sort === 'size') return a.size === b.size ? 0 : a.size === 0 ? 1 : b.size === 0 ? -1 : (a.size - b.size) * dir;
      const x = a[sort].toLowerCase(), y = b[sort].toLowerCase();
      if (x === y) return 0;
      if (x === '') return 1;
      if (y === '') return -1;
      return x.localeCompare(y, undefined, { numeric: true }) * dir;
    });
  });
  const by = (k: Key): void => {
    if (sort === k) desc = !desc;
    else {
      sort = k;
      desc = false;
    }
  };
  const COLS: { k: Key; label: string }[] = [{ k: 'name', label: 'wp.name' }, { k: 'version', label: 'wp.version' }, { k: 'publisher', label: 'wp.publisher' }, { k: 'installed', label: 'wp.installed' }, { k: 'size', label: 'wp.size' }];
</script>

{#if apps === null && loading}<p class="dim">{t('win.loading')}</p>
{:else if error}<div class="err">{error}</div>
{:else if apps}
  <div class="sp-fill">
    <div class="sp-tools"><SearchField placeholder={t('wp.search')} bind:value={q} /><span class="grow"></span><span class="dim">{rows.length} / {apps.length}</span>
      <button class="btn sm icon" type="button" title={t('win.refresh')} disabled={loading} onclick={load}><Icon name="refresh-cw" size={14} /></button></div>
    <div class="panel flush fill"><table class="tbl compact">
      <thead><tr>
        {#each COLS as c (c.k)}<th class="sortable" aria-sort={sort === c.k ? (desc ? 'descending' : 'ascending') : 'none'}><button type="button" onclick={() => by(c.k)}>{t(c.label)}{#if sort === c.k}<Icon name={desc ? 'arrow-down' : 'arrow-up'} size={12} />{/if}</button></th>{/each}
        <th>{t('wp.scope')}</th>
      </tr></thead>
      <tbody>
        {#each rows as a, i (`${a.name}|${a.version}|${a.scope}|${i}`)}
          <tr><td><b>{a.name}</b></td><td class="mono dim">{a.version || '—'}</td><td>{a.publisher || '—'}</td><td class="dim">{a.installed || '—'}</td><td class="mono">{a.size > 0 ? fmtBytes(a.size) : '—'}</td>
            <td><span class="badge {a.scope === 'machine' ? 'b-mute' : 'b-info'}">{t(`wp.scope.${a.scope}`)}{a.arch ? ` · ${a.arch}` : ''}</span></td></tr>
        {:else}<tr><td colspan="6" class="dim">{t('wp.empty')}</td></tr>{/each}
      </tbody>
    </table></div>
  </div>
{/if}
