<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { ApiError, serverApi, type DiskCleanItem, type DiskFS, type DiskNode, type DiskScan, type DiskState, type ServerDetail } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import { errorText } from '../errors';
  import Icon from '../Icon.svelte';
  import { i18n, t } from '../i18n.svelte';
  import PieChart, { type PiePart } from '../PieChart.svelte';
  import { fmtBytes } from '../status';
  import { showToast } from '../toast';

  let { s }: { s: ServerDetail } = $props();

  const POLL_MS = 60_000;
  const CATEGORY_COLOR: Record<string, string> = {
    sys: 'var(--accent)',
    docker: '#8b7cf6',
    logs: 'var(--warn)',
    db: 'var(--ok)',
    tmp: '#e0709a',
    home: '#4cc3d9',
    other: 'var(--off)',
    reserved: 'var(--border-hh)',
    free: 'var(--track)',
  };

  let info = $state<DiskState | null>(null);
  let phase = $state<'loading' | 'ready' | 'error'>('loading');
  let seq = 0;

  async function load(id: string): Promise<void> {
    const req = ++seq;
    try {
      const data = await serverApi.disk(id);
      if (req !== seq) return;
      info = data;
      phase = 'ready';
    } catch {
      if (req !== seq) return;
      phase = 'error';
    }
  }
  $effect(() => {
    const id = s.id;
    untrack(() => {
      info = null;
      scan = null;
      path = [];
      phase = 'loading';
      void load(id);
    });
    const timer = setInterval(() => void load(id), POLL_MS);
    return () => clearInterval(timer);
  });
  onMount(() => () => (seq += 1));

  const offline = $derived(!s.online);
  const filesystems = $derived(info?.filesystems ?? []);

  // ---- filesystems ----
  const usedColor = (pct: number): string => (pct >= 85 ? 'var(--crit)' : pct >= 70 ? 'var(--warn)' : 'var(--accent)');
  const fsParts = (f: DiskFS): PiePart[] => [
    { id: 'used', name: t('dk.used'), value: f.used, color: usedColor(f.pct) },
    { id: 'free', name: t('dk.free'), value: f.avail, color: 'var(--track)' },
  ];
  const days = (n: number): string => (n >= 365 ? t('dk.moreThanYear') : t('dk.inDays', { n: Math.max(1, Math.round(n)) }));
  const forecastTone = (f: DiskFS): string => (f.days_left > 0 && f.days_left <= 14 ? 'b-crit' : f.days_left > 0 && f.days_left <= 60 ? 'b-warn' : 'b-mute');
  function bars(trend: number[]): number[] {
    if (trend.length === 0) return [];
    const mx = Math.max(...trend);
    const mn = Math.min(...trend);
    return trend.map((v) => 18 + ((v - mn) / (mx - mn || 1)) * 82);
  }

  // ---- what takes the space ----
  let pick = $state('');
  let scan = $state<DiskScan | null>(null);
  let scanning = $state(false);
  let scanError = $state('');
  let path = $state<DiskNode[]>([]);

  const mount = $derived(filesystems.find((f) => f.mount === pick)?.mount ?? filesystems[0]?.mount ?? '');
  const mountFs = $derived(filesystems.find((f) => f.mount === mount) ?? null);
  const scanFor = $derived(scan && scan.mount === mount ? scan : null);
  const catName = (id: string): string => {
    const v = t(`dk.cat.${id}`);
    return v === `dk.cat.${id}` ? id : v; // an id the interface does not know
  };
  const categoryParts = $derived<PiePart[]>((scanFor?.categories ?? []).filter((c) => c.size > 0).map((c) => ({ id: c.id, name: catName(c.id), value: c.size, color: CATEGORY_COLOR[c.id] ?? 'var(--off)' })));

  async function runScan(force: boolean): Promise<void> {
    if (scanning || !mount) return;
    const id = s.id;
    scanning = true;
    scanError = '';
    try {
      const res = await serverApi.diskScan(id, mount, force);
      if (id !== s.id) return;
      scan = res;
      path = [res.tree];
    } catch (e) {
      scanError = errorText(e);
    } finally {
      scanning = false;
    }
  }
  function chooseMount(m: string): void {
    pick = m;
    path = scan && scan.mount === m ? [scan.tree] : [];
  }

  const current = $derived(path.length > 0 ? path[path.length - 1] : null);
  const rows = $derived.by(() => {
    const node = current;
    if (!node) return [];
    const kids = [...(node.children ?? [])].sort((a, b) => b.size - a.size);
    const rest = node.size - kids.reduce((n, k) => n + k.size, 0);
    const list: { node: DiskNode | null; name: string; size: number; link: boolean }[] = kids.map((k) => ({ node: k, name: k.name, size: k.size, link: (k.children?.length ?? 0) > 0 }));
    if (rest > 0) list.push({ node: null, name: t('dk.dirs.other'), size: rest, link: false });
    return list;
  });
  const biggest = $derived(rows.reduce((n, r) => Math.max(n, r.size), 1));
  const when = (sec: number): string => new Date(sec * 1000).toLocaleString(i18n.locale);

  // ---- cleanup ----
  let cleaning = $state<string | null>(null);
  let cleanError = $state('');
  const reclaimable = $derived((info?.cleanup ?? []).reduce((n, c) => n + c.frees, 0));

  /** What the agent runs, shown before the person agrees. */
  function command(c: DiskCleanItem): string {
    switch (c.kind) {
      case 'journal': return 'journalctl --vacuum-size=500M';
      case 'apt': return 'apt-get clean';
      case 'docker-images': return 'docker image prune -a';
      case 'docker-build': return 'docker builder prune';
      case 'win-temp': return 'Remove-Item C:\\Windows\\Temp\\*, C:\\Users\\*\\AppData\\Local\\Temp\\*';
      case 'win-update-cache': return 'Stop-Service wuauserv, bits; Remove-Item C:\\Windows\\SoftwareDistribution\\Download\\*';
      case 'win-recycle': return 'Clear-RecycleBin';
      case 'win-delivery': return 'Delete-DeliveryOptimizationCache';
      case 'win-dumps': return 'Remove-Item C:\\Windows\\Minidump\\*, C:\\Windows\\MEMORY.DMP';
      case 'win-old': return 'takeown /F C:\\Windows.old /R; Remove-Item C:\\Windows.old -Recurse';
      default: return `apt-get purge -y ${(c.items ?? []).join(' ')}`;
    }
  }

  async function clean(c: DiskCleanItem): Promise<void> {
    const list = c.items && c.items.length > 0 && c.kind !== 'kernels' ? `\n\n${c.items.join('\n')}` : '';
    const ok = await confirmAsk({
      title: t(`dk.clean.${c.kind}`),
      text: `${t('dk.clean.confirm', { what: t(`dk.clean.${c.kind}`), size: fmtBytes(c.frees) })}\n\n${command(c)}${list}\n\n${t('dk.clean.note')}`,
      ok: t('dk.clean.run'),
      danger: true,
    });
    if (!ok) return;
    cleaning = c.kind;
    cleanError = '';
    try {
      const res = await serverApi.diskClean(s.id, c.kind);
      showToast(t('dk.clean.done', { size: fmtBytes(res.freed) }));
      await load(s.id);
    } catch (e) {
      cleanError = e instanceof ApiError || e instanceof Error ? errorText(e) : t('common.genericError');
    } finally {
      cleaning = null;
    }
  }
</script>

<div class="disk">
  {#if offline}<p class="warnbox warn">{t('svc.offline')}</p>{/if}
  {#if phase === 'loading'}<p class="dim">{t('dash.loading')}</p>
  {:else if phase === 'error'}<div class="err" role="alert">{t('dk.loadFailed')}</div>{/if}

  {#if info}
    <div class="fss">
      {#each filesystems as f (f.mount)}
        <section class="panel">
          <h4 class="ph">{t('dk.partition')} <span class="mono">{f.mount}</span> <span class="dim sub">{f.device.replace('/dev/', '')} · {f.fs}</span></h4>
          <div class="fsrow">
            <PieChart parts={fsParts(f)} size={130} />
            <div class="fsinfo">
              <p class="dim">{fmtBytes(f.used)} / {fmtBytes(f.total)} · {Math.round(f.pct)}%{#if f.inodes_total > 0} · {t('dk.inodes', { pct: Math.round((f.inodes_used / f.inodes_total) * 100) })}{/if}</p>
              {#if f.history_days >= 0.5 && f.per_day !== 0}
                <p class="dim">{f.per_day > 0 ? t('dk.growth') : t('dk.shrink')} <b>{t('dk.perDay', { size: fmtBytes(Math.abs(f.per_day)) })}</b></p>
              {:else}<p class="dim">{t('dk.collecting')}</p>{/if}
              {#if f.days_left > 0}<p><span class="badge {forecastTone(f)}">{t('dk.fillsIn', { when: days(f.days_left) })}</span></p>{/if}
            </div>
          </div>
          {#if f.trend.length > 1}
            <div class="spark-bars" aria-label={t('dk.trend')}>{#each bars(f.trend) as h, i (i)}<i style:height="{h}%"></i>{/each}</div>
          {/if}
        </section>
      {/each}
    </div>

    <section class="panel">
      <div class="row between">
        <h4 class="ph" style="margin:0">{t('dk.what')}{#if mountFs} <span class="dim sub">{t('dk.ofTotal', { used: fmtBytes(mountFs.used), total: fmtBytes(mountFs.total) })}</span>{/if}</h4>
        <div class="row">
          {#if filesystems.length > 1}
            <div class="seg sm" role="group" aria-label={t('dk.partition')}>{#each filesystems as f (f.mount)}<button type="button" aria-pressed={mount === f.mount} onclick={() => chooseMount(f.mount)}>{f.mount}</button>{/each}</div>
          {/if}
          <button class="btn sm" type="button" disabled={scanning || offline || !mount} onclick={() => void runScan(scanFor !== null)}>{scanning ? t('dk.scanning') : scanFor ? t('dk.rescan') : t('dk.scan')}</button>
        </div>
      </div>
      {#if scanError}<div class="err" role="alert" style="margin-top:10px">{scanError}</div>{/if}
      <div style="margin-top:12px">
        {#if !scanFor}
          <p class="dim">{scanning ? t('dk.scanningHint') : t('dk.scanHint')}</p>
        {:else}
          <p class="dim" style="margin-bottom:10px">{t('dk.scannedAt', { time: when(scanFor.scanned_at) })}{#if scanFor.partial} · <span class="warnt">{t('dk.partial')}</span>{/if}</p>
          <PieChart parts={categoryParts} size={210} showShare />
        {/if}
      </div>
    </section>

    {#if scanFor && current}
      <section class="panel">
        <div class="row between">
          <h4 class="ph" style="margin:0">{t('dk.dirs')}</h4>
          <div class="crumbs">
            {#each path as n, i (i)}
              <button type="button" onclick={() => (path = path.slice(0, i + 1))}>{n.name}</button>{#if i < path.length - 1}<Icon name="chevron-right" size={12} />{/if}
            {/each}
          </div>
        </div>
        <div class="dir-rows">
          {#each rows as r (r.name)}
            {#if r.link}
              <button class="drow link" type="button" onclick={() => (path = [...path, r.node!])}>
                <span class="nm mono"><span class="t">{r.name}</span><Icon name="chevron-right" size={12} /></span><span class="bar lv-ok"><i style:width="{(r.size / biggest) * 100}%"></i></span><span class="sz mono">{fmtBytes(r.size)}</span>
              </button>
            {:else}
              <div class="drow"><span class="nm mono"><span class="t">{r.name}</span></span><span class="bar lv-ok"><i style:width="{(r.size / biggest) * 100}%"></i></span><span class="sz mono">{fmtBytes(r.size)}</span></div>
            {/if}
          {:else}<p class="dim">{t('dk.dirs.empty')}</p>{/each}
        </div>
        <p class="dim" style="margin-top:8px">{t('dk.dirs.hint')}</p>
      </section>
    {/if}

    <section class="panel">
      <div class="row between">
        <h4 class="ph" style="margin:0">{t('dk.clean')}</h4>
        {#if reclaimable > 0}<span class="dim">{t('dk.clean.upTo', { size: fmtBytes(reclaimable) })}</span>{/if}
      </div>
      {#if cleanError}<div class="err" role="alert" style="margin-top:10px">{cleanError}</div>{/if}
      {#if info.cleanup.length === 0}
        <p class="dim" style="margin-top:10px">{t('dk.clean.none')}</p>
      {:else}
        <div class="clean">
          {#each info.cleanup as c (c.kind)}
            <div class="cc">
              <span class="dim">{t(`dk.clean.${c.kind}`)}</span>
              <b class="big">{fmtBytes(c.size)}</b>
              <span class="dim">{t(`dk.clean.${c.kind}.text`, { frees: fmtBytes(c.frees), n: c.count ?? 0 })}</span>
              <div class="row end"><button class="btn sm" type="button" disabled={offline || cleaning !== null} onclick={() => void clean(c)}>{cleaning === c.kind ? t('dk.cleaning') : t(`dk.clean.${c.kind}.go`)}</button></div>
            </div>
          {/each}
        </div>
      {/if}
    </section>
  {/if}
</div>
