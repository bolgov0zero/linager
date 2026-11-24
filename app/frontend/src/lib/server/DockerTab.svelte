<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi2, type DockerContainer, type DockerEditConfig, type DockerImage, type DockerInfo, type ServerDetail } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import ContextMenu, { type CtxItem } from '../ContextMenu.svelte';
  import { confirmAsk } from '../dialogs.svelte';
  import { cpuText, errorText as dockerError, groupContainers, imageTitle, isRunning, ramText, shortId, stateTone, unavailable, type ContainerGroup } from '../docker-util';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import { fmtBytes } from '../status';
  import DockerEditModal from './DockerEditModal.svelte';
  import StreamModal from './StreamModal.svelte';

  let { s }: { s: ServerDetail } = $props();

  type Section = 'containers' | 'images';
  interface Stream { kind: 'logs' | 'pull' | 'update'; title: string; sk: string; params: Record<string, string | number> }

  let section = $state<Section>('containers');
  let info = $state<DockerInfo | null>(null);
  let loadState = $state<'loading' | 'ready' | 'offline' | 'disabled' | 'error'>('loading');
  let loadError = $state('');
  let busy = $state<string | null>(null);
  let stream = $state<Stream | null>(null);
  let updateFor = $state<{ c: DockerContainer; image: string } | null>(null);
  let editFor = $state<{ c: DockerContainer; config: DockerEditConfig } | null>(null);
  let openGroups = $state<Record<string, boolean>>({});
  let checking = $state(false);
  let pullInput = $state('');
  let pullError = $state('');
  let menu = $state<{ x: number; y: number; items: CtxItem[] } | null>(null);
  let note = $state<{ text: string; bad: boolean } | null>(null);
  let noteTimer: ReturnType<typeof setTimeout> | undefined;
  let seq = 0;

  const sid = $derived(s.id);
  const online = $derived(s.online);
  const containers = $derived<DockerContainer[]>(info?.containers ?? []);
  const images = $derived<DockerImage[]>(info?.images ?? []);
  const groups = $derived(groupContainers(containers));
  const hasProjects = $derived(groups.some((g) => g.key !== ''));
  const updatable = $derived(containers.filter((c) => c.upd === 'new' && !c.ignored));
  const readOnly = $derived(!online || loadState === 'offline' || loadState === 'disabled');
  const runningCount = $derived(containers.filter(isRunning).length);

  function say(text: string, bad = false): void {
    note = { text, bad };
    clearTimeout(noteTimer);
    noteTimer = setTimeout(() => (note = null), bad ? 8000 : 4000);
  }

  async function refresh(): Promise<void> {
    const my = ++seq;
    try {
      const data = await serverApi2.docker(sid);
      if (my !== seq) return;
      info = data;
      loadState = 'ready';
      loadError = '';
    } catch (e) {
      if (my !== seq) return;
      const kind = unavailable(e);
      if (kind) return void (loadState = kind);
      loadState = 'error';
      loadError = dockerError(e, t);
    }
  }
  onMount(() => {
    void refresh();
    const i = setInterval(() => void refresh(), 5000);
    return () => clearInterval(i);
  });

  const isOpen = (g: ContainerGroup): boolean => openGroups[g.key] ?? g.key === '';
  const rowBusy = (c: DockerContainer): boolean => busy !== null && busy.startsWith(`${c.id}:`);

  async function containerAction(c: DockerContainer, action: 'start' | 'stop' | 'restart' | 'remove'): Promise<void> {
    busy = `${c.id}:${action}`;
    try {
      await serverApi2.dockerContainerAction(sid, c.id, action);
      say(t('docker.done', { action: t(`docker.act.${action}`), name: c.name }));
      await refresh();
    } catch (e) {
      say(dockerError(e, t), true);
    } finally {
      busy = null;
    }
  }
  async function removeContainer(c: DockerContainer): Promise<void> {
    if (await confirmAsk({ title: t('docker.removeTitle'), text: t('docker.removeText', { name: c.name }), ok: t('docker.act.remove'), danger: true })) await containerAction(c, 'remove');
  }
  async function removeImage(img: DockerImage): Promise<void> {
    if (!(await confirmAsk({ title: t('docker.removeImageTitle'), text: t('docker.removeImageText', { name: imageTitle(img) }), ok: t('docker.act.remove'), danger: true }))) return;
    try {
      await serverApi2.dockerRemoveImage(sid, img.id);
      say(t('docker.imageDeleted'));
      await refresh();
    } catch (e) {
      say(dockerError(e, t), true);
    }
  }
  async function toggleIgnore(c: DockerContainer): Promise<void> {
    try {
      await serverApi2.dockerIgnore(sid, c.name, !c.ignored);
      await refresh();
    } catch (e) {
      say(dockerError(e, t), true);
    }
  }
  async function checkImages(): Promise<void> {
    if (checking || readOnly) return;
    checking = true;
    try {
      const res = await serverApi2.dockerCheck(sid);
      say(t('docker.check.done', { n: res.new }));
      await refresh();
    } catch (e) {
      say(dockerError(e, t), true);
    } finally {
      checking = false;
    }
  }
  async function requestEdit(c: DockerContainer): Promise<void> {
    if (busy !== null || editFor) return;
    busy = `${c.id}:edit`;
    try {
      editFor = { c, config: await serverApi2.dockerConfig(sid, c.id) };
    } catch (e) {
      say(dockerError(e, t), true);
    } finally {
      busy = null;
    }
  }

  // ---- streams (logs, pull, update, edit) ----
  const logs = (c: DockerContainer): void => void (stream = { kind: 'logs', title: t('docker.logsTitle', { name: c.name }), sk: 'docker-logs', params: { container: c.id, lines: 200 } });
  function startPull(): void {
    const image = pullInput.trim();
    if (!image) return void (pullError = t('docker.pull.emptyError'));
    pullError = '';
    stream = { kind: 'pull', title: t('docker.pull.title', { image }), sk: 'docker-pull', params: { image } };
  }
  function startUpdate(): void {
    const u = updateFor;
    if (!u) return;
    updateFor = null;
    const params: Record<string, string> = { container: u.c.id };
    if (!u.c.compose && u.image.trim() && u.image.trim() !== u.c.image) params.image = u.image.trim();
    stream = { kind: 'update', title: t('docker.update.streamTitle', { name: u.c.name }), sk: 'docker-update', params };
  }
  async function updateMarked(): Promise<void> {
    const list = updatable;
    if (list.length === 0) return;
    if (!(await confirmAsk({ title: t('docker.marked.title'), text: t('docker.marked.text', { names: list.map((c) => c.name).join(', ') }), ok: t('docker.update.go') }))) return;
    stream = { kind: 'update', title: t('docker.marked.title'), sk: 'docker-update', params: { containers: list.map((c) => c.id).join(',') } };
  }
  function openProject(g: ContainerGroup, what: 'logs' | 'update'): void {
    if (what === 'logs') stream = { kind: 'logs', title: t('docker.grp.logsTitle', { name: g.key }), sk: 'docker-project-logs', params: { container: g.items[0].id, lines: 200 } };
    else void confirmAsk({ title: t('docker.grp.updateTitle', { name: g.key }), text: t('docker.grp.updateText'), ok: t('docker.update.go') }).then((ok) => {
      if (ok) stream = { kind: 'update', title: t('docker.grp.updateTitle', { name: g.key }), sk: 'docker-update', params: { container: g.items[0].id, project: 1 } };
    });
  }
  async function projectAction(g: ContainerGroup, action: 'start' | 'restart'): Promise<void> {
    busy = `group:${g.key}`;
    try {
      for (const c of g.items) {
        if (action === 'restart' && !isRunning(c)) continue;
        if (action === 'start' && isRunning(c)) continue;
        await serverApi2.dockerContainerAction(sid, c.id, action);
      }
      say(t('docker.grp.done', { name: g.key, action: t(`docker.act.${action}`) }));
      await refresh();
    } catch (e) {
      say(dockerError(e, t), true);
    } finally {
      busy = null;
    }
  }
  function closeStream(): void {
    stream = null;
    void refresh();
  }
  function startEdit(token: string): void {
    const c = editFor?.c;
    editFor = null;
    if (c) stream = { kind: 'update', title: t('dockerEdit.streamTitle', { name: c.name }), sk: 'docker-edit', params: { token } };
  }

  // ---- menus ----
  function containerItems(c: DockerContainer): CtxItem[] {
    const running = isRunning(c);
    const b = readOnly || rowBusy(c);
    return [
      { key: 'logs', label: t('docker.logs'), icon: 'scroll-text', disabled: readOnly, onselect: () => logs(c) },
      running ? { key: 'stop', label: t('docker.act.stop'), icon: 'square', disabled: b, onselect: () => void containerAction(c, 'stop') } : { key: 'start', label: t('docker.act.start'), icon: 'play', disabled: b, onselect: () => void containerAction(c, 'start') },
      { key: 'restart', label: t('docker.act.restart'), icon: 'rotate-ccw', disabled: b || !running, onselect: () => void containerAction(c, 'restart') },
      { key: 'update', label: t('docker.act.update'), icon: 'download', separated: true, disabled: b, onselect: () => (updateFor = { c, image: c.image }) },
      { key: 'edit', label: t('docker.act.edit'), icon: 'pencil', disabled: b, onselect: () => void requestEdit(c) },
      { key: 'ignore', label: c.ignored ? t('docker.ignore.off') : t('docker.ignore.on'), icon: 'eye', disabled: readOnly, onselect: () => void toggleIgnore(c) },
      { key: 'remove', label: t('docker.act.remove'), icon: 'trash-2', danger: true, separated: true, disabled: b, onselect: () => void removeContainer(c) },
    ];
  }
  function openMenu(items: CtxItem[], ev: MouseEvent): void {
    ev.preventDefault();
    ev.stopPropagation();
    const r = (ev.currentTarget as HTMLElement).getBoundingClientRect();
    menu = { x: ev.type === 'contextmenu' ? ev.clientX : r.left, y: ev.type === 'contextmenu' ? ev.clientY : r.bottom + 2, items };
  }
  const checkedText = (at: number | undefined): string => (at ? t('docker.check.at', { time: new Date(at * 1000).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' }) }) : t('docker.check.never'));
  const tone = (st: string): string => ({ good: 'b-ok', warn: 'b-warn', crit: 'b-crit', muted: 'b-mute' })[stateTone(st)];
</script>

{#snippet containerRow(c: DockerContainer)}
  {@const running = isRunning(c)}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <tr class:hl={c.upd === 'new' && !c.ignored} oncontextmenu={(e) => openMenu(containerItems(c), e)}>
    <td><b class="mono">{c.name}</b>{#if c.project && !hasProjects} <span class="dim">· {c.project}</span>{/if}<div class="dim mono sub" title={c.image}>{c.image}</div></td>
    <td><span class="badge {tone(c.state)}">{c.state}</span></td>
    <td class="mono dim" title={c.ports.join(', ')}>{c.ports.join(', ') || '—'}</td>
    <td class="mono r">{cpuText(c)}</td><td class="mono r">{ramText(c)}</td>
    <td class="dim">{c.status}</td>
    <td>{#if c.upd === 'new'}<span class="badge {c.ignored ? 'b-mute' : 'b-warn'}">{c.ignored ? t('docker.badge.ignored') : t('docker.upd.new')}</span>{#if c.upd_local && c.upd_remote} <span class="dim mono">{c.upd_local} → {c.upd_remote}</span>{/if}
      {:else if c.upd === 'current'}<span class="badge b-ok">{t('docker.upd.current')}</span>
      {:else if c.upd === 'unknown'}<span class="badge b-mute" title={c.upd_error || t('docker.upd.unknownHint')}>{t('docker.upd.unknown')}</span>
      {:else}<span class="dim">{t('docker.upd.none')}</span>{/if}</td>
    <td class="r"><span class="r-btns">
      <button class="btn icon sm" type="button" title={t('docker.logs')} disabled={readOnly} onclick={() => logs(c)}><Icon name="scroll-text" size={14} /></button>
      {#if running}<button class="btn icon sm danger" type="button" title={t('docker.act.stop')} disabled={readOnly || rowBusy(c)} onclick={() => containerAction(c, 'stop')}><Icon name="square" size={14} /></button>
      {:else}<button class="btn icon sm" type="button" title={t('docker.act.start')} disabled={readOnly || rowBusy(c)} onclick={() => containerAction(c, 'start')}><Icon name="play" size={14} /></button>{/if}
      <button class="btn icon sm" type="button" title={t('docker.menu')} aria-label={t('docker.menu')} disabled={readOnly} onclick={(e) => openMenu(containerItems(c), e)}><Icon name="ellipsis" size={14} /></button></span></td>
  </tr>
{/snippet}

{#if !info}
  {#if loadState === 'error'}<div class="err">{t('docker.loadFailed')}: {loadError} <button class="btn sm" type="button" onclick={refresh}>{t('docker.retry')}</button></div>
  {:else if loadState === 'offline'}<p class="dim">{t('docker.offline')}</p>
  {:else if loadState === 'disabled'}<p class="dim">{t('docker.disabled')}</p>
  {:else}<p class="dim">{t('dash.loading')}</p>{/if}
{:else if !info.available}<p class="dim">{t('docker.unavailable')}</p>{:else}
  <div class="docker-page">
    <div class="row">
      <button class="chip" type="button" aria-pressed={section === 'containers'} onclick={() => (section = 'containers')}>{t('docker.tabs.containers')}<em>{containers.length}</em></button>
      <button class="chip" type="button" aria-pressed={section === 'images'} onclick={() => (section = 'images')}>{t('docker.tabs.images')}<em>{images.length}</em></button>
      <span class="grow"></span>
      {#if section === 'containers'}
        <span class="dim" title={t('docker.check.help')}>{checkedText(info.checked_at)}{#if info.checked_at && updatable.length > 0}{' · '}{t('docker.check.avail', { n: updatable.length })}{/if}</span>
        <button class="btn sm" type="button" disabled={readOnly || checking} onclick={checkImages}><Icon name="refresh-cw" size={14} />{checking ? t('docker.check.busy') : t('docker.check.btn')}</button>
        <button class="btn sm primary" type="button" disabled={readOnly || updatable.length === 0} onclick={updateMarked}><Icon name="download" size={14} />{t('docker.updateMarked')}</button>
      {:else}<span class="dim">{t('docker.summary', { running: runningCount, total: containers.length })}</span>{/if}
    </div>
    {#if note}<div class="fm-note" class:bad={note.bad}><span>{note.text}</span></div>{/if}
    {#if readOnly}<div class="banner">{t(online ? 'docker.disabled' : 'docker.offline')}</div>{:else if loadState === 'error'}<div class="banner">{t('docker.refreshFailed')}: {loadError}</div>{/if}

    {#if section === 'containers'}
      {#if containers.length === 0}<p class="dim">{t('docker.noContainers')}</p>{:else}
        <div class="tw" style="max-height:none"><table class="tbl compact">
          <thead><tr><th>{t('docker.col.container')}</th><th>{t('docker.col.state')}</th><th>{t('docker.col.ports')}</th><th class="r">{t('docker.col.cpu')}</th><th class="r">{t('docker.col.ram')}</th><th>{t('docker.col.status')}</th><th>{t('docker.col.registry')}</th><th></th></tr></thead>
          <tbody>
            {#each groups as g (g.key)}
              {#if hasProjects && g.key !== ''}
                <tr class="grp-h"><td colspan="8">
                  <div class="row">
                    <button class="btn xs" type="button" onclick={() => (openGroups[g.key] = !isOpen(g))} aria-label={g.key}><Icon name={isOpen(g) ? 'chevron-down' : 'chevron-right'} size={14} /></button>
                    <b>{g.key}</b><span class="dim">compose · {t('docker.grp.count', { n: g.items.length })} · {g.running === g.items.length ? t('docker.grp.allRunning') : t('docker.grp.running', { running: g.running, total: g.items.length })}</span>
                    {#if g.updatable > 0}<span class="badge b-warn">{t('docker.grp.updatable', { n: g.updatable })}</span>{/if}
                    {#if g.file}<span class="dim mono">{g.file}</span>{/if}
                    <span class="grow"></span>
                    <button class="btn sm" type="button" disabled={readOnly} onclick={() => openProject(g, 'update')}>{t('docker.grp.update')}</button>
                    {#if g.running > 0}<button class="btn sm" type="button" disabled={readOnly || busy === `group:${g.key}`} onclick={() => projectAction(g, 'restart')}>{t('docker.grp.restart')}</button>{/if}
                    {#if g.running < g.items.length}<button class="btn sm" type="button" disabled={readOnly || busy === `group:${g.key}`} onclick={() => projectAction(g, 'start')}>{t('docker.grp.start')}</button>{/if}
                    <button class="btn sm" type="button" disabled={readOnly} onclick={() => openProject(g, 'logs')}>{t('docker.grp.logs')}</button>
                  </div>
                </td></tr>
              {:else if hasProjects}<tr class="grp-h"><td colspan="8"><b class="dim">{t('docker.grp.plain')}</b></td></tr>{/if}
              {#if isOpen(g)}{#each g.items as c (c.id)}{@render containerRow(c)}{/each}{/if}
            {/each}
          </tbody>
        </table></div>
      {/if}
    {:else}
      {#if images.length === 0}<p class="dim">{t('docker.noImages')}</p>{:else}
        <div class="tw" style="max-height:none"><table class="tbl compact">
          <thead><tr><th>{t('docker.col.image')}</th><th>ID</th><th class="r">{t('docker.col.size')}</th><th>{t('docker.col.created')}</th><th></th></tr></thead>
          <tbody>{#each images as img (img.id)}
            <tr><td class="mono"><b>{imageTitle(img)}</b>{#if img.used} <span class="badge b-info">{t('docker.used')}</span>{/if}{#if (img.repo_tags ?? []).length > 1}<div class="dim sub">{img.repo_tags.slice(1).join(', ')}</div>{/if}</td>
              <td class="mono dim">{shortId(img.id)}</td><td class="mono r">{fmtBytes(img.size)}</td><td class="dim">{img.created ? new Date(img.created * 1000).toLocaleString() : '—'}</td>
              <td class="r"><button class="btn icon sm danger" type="button" title={img.used ? t('docker.used') : t('docker.act.remove')} disabled={readOnly || img.used} onclick={() => removeImage(img)}><Icon name="trash-2" size={14} /></button></td></tr>
          {/each}</tbody>
        </table></div>
      {/if}
      <section class="panel">
        <form class="row" onsubmit={(e) => { e.preventDefault(); startPull(); }}>
          <label class="fl grow"><span>{t('docker.pull.label')}</span><span class="field"><input class="mono" placeholder="nginx:1.27" spellcheck="false" autocomplete="off" bind:value={pullInput} oninput={() => (pullError = '')} /></span></label>
          <button class="btn primary" type="submit" disabled={readOnly} style="align-self:flex-end">{t('docker.pull.submit')}</button>
        </form>
        {#if pullError}<div class="err">{pullError}</div>{/if}
      </section>
    {/if}
  </div>
{/if}

{#if menu}<ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />{/if}
{#if stream}<StreamModal title={stream.title} serverId={sid} kind={stream.sk} params={stream.params} onend={() => stream?.kind !== 'logs' && void refresh()} onclose={closeStream} />{/if}
{#if updateFor}
  <Modal title={t('docker.update.title', { name: updateFor.c.name })} onclose={() => (updateFor = null)}>
    <p>{t('docker.update.text')}</p>
    {#if updateFor.c.compose}<p class="dim">{t('docker.update.compose')}</p>
    {:else}<label class="fl"><span>{t('docker.update.image')}</span><span class="field"><input class="mono" spellcheck="false" bind:value={updateFor.image} /></span></label>{/if}
    {#snippet foot()}<span class="spacer"></span><button class="btn" type="button" onclick={() => (updateFor = null)}>{t('common.cancel')}</button><button class="btn primary" type="button" onclick={startUpdate}>{t('docker.update.submit')}</button>{/snippet}
  </Modal>
{/if}
{#if editFor}<DockerEditModal serverId={sid} container={editFor.c} config={editFor.config} others={containers.filter((x) => x.id !== editFor?.c.id)} onclose={() => (editFor = null)} onapply={startEdit} />{/if}
