<script lang="ts">
  import { ApiError, winCall, type ServerDetail, type WinApp } from '../../../api';
  import { tr as T } from '../../../i18n';
  import { formatBytes } from '../format';

  let { server }: { server: ServerDetail } = $props();
  const serverId = $derived(server.id);

  type SortKey = 'name' | 'version' | 'publisher' | 'installed' | 'size';

  let apps = $state<WinApp[] | null>(null);
  let loading = $state(false);
  let error = $state('');
  let q = $state('');
  let sort = $state<SortKey>('name');
  let desc = $state(false);

  async function load(): Promise<void> {
    loading = true;
    try {
      const r = await winCall<{ apps: WinApp[] }>(serverId, 'apps', 'list');
      apps = r.apps;
      error = '';
    } catch (e) {
      // 404 / 501: an agent that predates the module
      error = e instanceof ApiError && (e.status === 404 || e.status === 501) ? $T('wp.old') : e instanceof ApiError && e.serverMessage ? e.serverMessage : $T('win.error');
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void serverId;
    void load();
  });

  const rows = $derived.by(() => {
    const needle = q.trim().toLowerCase();
    const list = (apps ?? []).filter((a) => !needle || `${a.name} ${a.publisher}`.toLowerCase().includes(needle));
    const dir = desc ? -1 : 1;
    return list.sort((a, b) => {
      // an empty value (no date, no size) goes to the end whichever the direction is
      if (sort === 'size') return a.size === b.size ? 0 : a.size === 0 ? 1 : b.size === 0 ? -1 : (a.size - b.size) * dir;
      const x = a[sort].toLowerCase();
      const y = b[sort].toLowerCase();
      if (x === y) return 0;
      if (x === '') return 1;
      if (y === '') return -1;
      return x.localeCompare(y, undefined, { numeric: true }) * dir;
    });
  });

  function by(key: SortKey): void {
    if (sort === key) desc = !desc;
    else {
      sort = key;
      desc = false;
    }
  }

  const COLS: { key: SortKey; label: string }[] = [
    { key: 'name', label: 'wp.name' },
    { key: 'version', label: 'wp.version' },
    { key: 'publisher', label: 'wp.publisher' },
    { key: 'installed', label: 'wp.installed' },
    { key: 'size', label: 'wp.size' },
  ];
</script>

<div class="wrap">
  <div class="tools">
    <input class="input search" type="search" bind:value={q} placeholder={$T('wp.search')} aria-label={$T('wp.search')} />
    <button class="btn" type="button" disabled={loading} onclick={() => void load()}>{$T('win.refresh')}</button>
  </div>
  {#if apps === null && loading}
    <p class="muted">{$T('win.loading')}</p>
  {:else if error}
    <p class="err" role="alert">{error}</p>
  {:else if apps}
    <div class="table-wrap">
      <table class="tbl">
        <thead>
          <tr>
            {#each COLS as c (c.key)}
              <th scope="col" aria-sort={sort === c.key ? (desc ? 'descending' : 'ascending') : 'none'}>
                <button class="th" type="button" onclick={() => by(c.key)}>{$T(c.label)}{#if sort === c.key}<span class="arr" aria-hidden="true">{desc ? '↓' : '↑'}</span>{/if}</button>
              </th>
            {/each}
            <th scope="col">{$T('wp.scope')}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as a, i (`${a.name}|${a.version}|${a.scope}|${i}`)}
            <tr>
              <td class="nm">{a.name}</td>
              <td class="nw mono">{a.version || '—'}</td>
              <td>{a.publisher || '—'}</td>
              <td class="nw">{a.installed || '—'}</td>
              <td class="nw">{a.size > 0 ? formatBytes(a.size) : '—'}</td>
              <td class="nw"><span class="badge" class:tone-muted={a.scope === 'machine'}>{$T(`wp.scope.${a.scope}`)}{a.arch ? ` · ${a.arch}` : ''}</span></td>
            </tr>
          {:else}
            <tr><td colspan="6" class="muted empty">{$T('wp.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
    <p class="muted small">{$T('wp.legend', { shown: rows.length, total: apps.length })}</p>
  {/if}
</div>

<style>
  .wrap { display: flex; flex-direction: column; gap: 10px; }
  .tools { display: flex; flex-wrap: wrap; align-items: center; gap: 7px; }
  .search { flex: 1 1 200px; max-width: 288px; height: 29px; padding-block: 0; font-size: 12px; }
  .table-wrap { overflow-x: auto; }
  .tbl { width: 100%; border-collapse: collapse; font-size: 13px; }
  .tbl th { padding: 0; text-align: left; font-family: var(--font-display); font-weight: 400; font-size: 12px; color: var(--muted); border-bottom: 2px solid var(--line); white-space: nowrap; }
  .th { padding: 6px 8px; background: none; border: 0; font: inherit; color: inherit; cursor: pointer; }
  .th:hover { color: var(--ink); }
  .arr { margin-left: 4px; }
  .tbl th:last-child { padding: 6px 8px; }
  .tbl td { padding: 8px; vertical-align: middle; border-bottom: 1px dashed var(--seg-off); }
  .tbl tbody tr:hover td { background: var(--seg-off); }
  .nm { font-weight: 600; overflow-wrap: anywhere; }
  .nw { white-space: nowrap; }
  .badge { display: inline-block; padding: 0 5px; border: 2px solid currentColor; font-size: 11px; line-height: 1.35; white-space: nowrap; }
  .tone-muted { color: var(--muted); }
  .empty { text-align: center; }
  .err { color: var(--crit); }
  p { margin: 0; }
</style>
