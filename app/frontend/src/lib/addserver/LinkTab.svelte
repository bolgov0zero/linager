<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type Group, type JoinCommand, type JoinLink, type JoinLinkCreated } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { confirmAsk } from '../dialogs.svelte';
  import { isUnauthorized } from '../errors';
  import { t } from '../i18n.svelte';
  import Fold from '../settings/Fold.svelte';
  import { fmtDateTime } from '../settings/upd-format';
  import { showToast } from '../toast';
  import { fmtIn, joinScriptText } from './link';

  let { active, groups, onauthlost }: { active: boolean; groups: Group[]; onauthlost: () => void } = $props();

  const POLL_MS = 5000;
  const TTLS = [900, 3600, 86400, 604800];
  const USES = [1, 5, 10, 25, 0];

  let ttl = $state(3600);
  let uses = $state(1);
  let groupId = $state(0);
  let nets = $state('');
  let auto = $state(false);
  let busy = $state(false);
  let formError = $state('');

  /** The link just made: the token and the commands live only here, in memory, and are never stored anywhere. */
  let created = $state<JoinLinkCreated | null>(null);
  let createdWith = $state({ ttl: 3600, uses: 1, auto: false });
  let showScript = $state(false);
  let fingerprint = $state('');

  let links = $state<JoinLink[]>([]);
  let linksOpen = $state(false);
  let revoking = $state<Record<number, boolean>>({});
  let now = $state(Date.now());
  let platform = $state<'linux' | 'windows'>('linux');

  const ttlOptions = $derived(TTLS.map((v) => ({ v: String(v), t: t(`asv.link.ttl.${v}`) })));
  const usesOptions = $derived(USES.map((v) => ({ v: String(v), t: v === 0 ? t('asv.link.uses.0') : String(v) })));
  const groupOptions = $derived([{ v: '0', t: t('asv.group.none') }, ...groups.map((g) => ({ v: String(g.id), t: g.name }))]);
  const commands = $derived(created?.commands.filter((c) => (c.kind === 'external' || c.kind === 'local') && (c.platform ?? 'linux') === platform) ?? []);
  const scriptText = $derived(created && platform === 'linux' && commands.length > 0 ? joinScriptText(created.token, commands[0].command, fingerprint) : '');

  function parseNets(text: string): string[] | null {
    const list = text.split(',').map((s) => s.trim()).filter(Boolean);
    return list.every((n) => /^[0-9a-f:.]+(\/\d{1,3})?$/i.test(n)) ? list : null;
  }
  async function loadLinks(): Promise<void> {
    try {
      links = await api.joinLinks();
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
    }
  }
  async function create(): Promise<void> {
    if (busy) return;
    formError = '';
    const list = parseNets(nets);
    if (list === null) {
      formError = t('asv.link.netsBad');
      return;
    }
    busy = true;
    try {
      created = await api.joinLinkCreate({ ttl_seconds: ttl, max_uses: uses, auto_approve: auto, networks: list, group_id: groupId === 0 ? null : groupId });
      createdWith = { ttl, uses, auto };
      showScript = false;
      showToast(t('asv.link.createdToast'));
      if (!fingerprint) {
        try {
          fingerprint = (await api.connectInfo()).fingerprint;
        } catch {
          // the script is shown without the fingerprint line
        }
      }
      await loadLinks();
    } catch (e) {
      if (isUnauthorized(e)) return onauthlost();
      if (e instanceof ApiError && e.status === 422) formError = e.code ?? t('common.genericError');
      else showToast(t('common.genericError'));
    } finally {
      busy = false;
    }
  }
  async function copy(cmd: JoinCommand): Promise<void> {
    try {
      await navigator.clipboard.writeText(cmd.command);
      showToast(t('st.common.copied'));
    } catch {
      showToast(t('st.common.copyFailed'));
    }
  }
  async function revoke(link: JoinLink): Promise<void> {
    if (revoking[link.id]) return;
    if (!(await confirmAsk({ title: t('asv.link.revoke'), text: link.label, ok: t('asv.link.revoke'), danger: true }))) return;
    revoking[link.id] = true;
    try {
      await api.joinLinkRevoke(link.id);
      if (created?.id === link.id) created = null;
      showToast(t('asv.link.revoked'));
      await loadLinks();
    } catch (e) {
      if (isUnauthorized(e)) return onauthlost();
      showToast(t('common.genericError'));
    } finally {
      revoking[link.id] = false;
    }
  }
  const expiresText = (link: JoinLink): string => {
    const left = link.expires_at - Math.floor(now / 1000);
    return left <= 0 ? t('asv.link.expired') : t('asv.link.in', { t: fmtIn(left) });
  };
  const usedText = (link: JoinLink): string => t('asv.link.usedOf', { n: link.uses, max: link.max_uses === 0 ? '∞' : link.max_uses });

  onMount(() => {
    void loadLinks();
    const timer = setInterval(() => {
      now = Date.now();
      if (active && !document.hidden) void loadLinks();
    }, POLL_MS);
    return () => clearInterval(timer);
  });
</script>

<div class="asv-tab">
  <h3>{t('asv.tab.link')}</h3>
  <p class="dim">{t('asv.link.lead')}</p>

  <div class="asv-sec">
    <h4 class="ph">{t('asv.link.new')}</h4>
    <div class="panel mdl-stack">
      <div class="asv-grid">
        <div class="fl"><span>{t('asv.link.ttl')}</span><CustomSelect value={String(ttl)} options={ttlOptions} onchange={(v) => (ttl = Number(v))} /></div>
        <div class="fl"><span>{t('asv.link.uses')}</span><CustomSelect value={String(uses)} options={usesOptions} onchange={(v) => (uses = Number(v))} /></div>
        <div class="fl"><span>{t('asv.link.group')}</span><CustomSelect value={String(groupId)} options={groupOptions} onchange={(v) => (groupId = Number(v))} /></div>
      </div>
      <label class="fl"><span>{t('asv.link.nets')}</span><span class="field"><input class="mono" bind:value={nets} placeholder={t('asv.link.netsPlaceholder')} spellcheck="false" autocomplete="off" oninput={() => (formError = '')} /></span><small class="dim">{t('asv.link.netsHint')}</small></label>
      <label class="sw-row"><span><b>{t('asv.link.auto')}</b><small>{t('asv.link.autoHint')}</small></span><span class="sw"><input type="checkbox" role="switch" bind:checked={auto} /><i></i></span></label>
      {#if formError}<div class="err" role="alert">{formError}</div>{/if}
      <div class="row end"><button class="btn primary" type="button" disabled={busy} onclick={() => void create()}>{t('asv.link.create')}</button></div>
    </div>
  </div>

  {#if created}
    <div class="asv-sec">
      <h4 class="ph">{t('asv.link.created')}</h4>
      <div class="panel mdl-stack">
        <p><b>{t('asv.link.once')}</b> {t('asv.link.onceRest', { ttl: t(`asv.link.ttl.${createdWith.ttl}`), max: createdWith.uses === 0 ? t('asv.link.uses.0') : createdWith.uses, mode: createdWith.auto ? t('asv.link.modeAuto') : t('asv.link.modeManual') })}</p>
        <div class="seg sm" aria-label={t('asv.link.platform')}>
          <button type="button" aria-pressed={platform === 'linux'} onclick={() => (platform = 'linux')}>Linux</button>
          <button type="button" aria-pressed={platform === 'windows'} onclick={() => (platform = 'windows')}>Windows</button>
        </div>
        {#if platform === 'windows'}<p class="dim">{t('asv.link.winHint')}</p>{/if}
        {#each commands as cmd (cmd.kind)}
          <div class="mdl-stack">
            <div class="row between"><b>{t(cmd.kind === 'external' ? 'asv.link.ext' : 'asv.link.loc')}</b><button class="btn sm" type="button" onclick={() => void copy(cmd)}>{t('st.common.copy')}</button></div>
            <p class="dim">{t(cmd.kind === 'external' ? 'asv.link.extHint' : 'asv.link.locHint')}</p>
            <pre class="json mono">{cmd.command}</pre>
          </div>
        {/each}
        {#if platform === 'linux'}
          <div class="row between"><button class="btn sm" type="button" onclick={() => (showScript = !showScript)}>{t(showScript ? 'asv.link.hideScript' : 'asv.link.showScript')}</button><span class="dim">{t('asv.link.scriptHint')}</span></div>
          {#if showScript}<pre class="json mono">{scriptText}</pre>{/if}
        {/if}
        <p class="warnbox warn"><b>{t('asv.link.whyTitle')}</b> {t('asv.link.why')}</p>
      </div>
    </div>
  {/if}

  <Fold title={t('asv.link.list')} meta={String(links.length)} bind:open={linksOpen}>
    {#if links.length === 0}<p class="dim">{t('asv.link.listEmpty')}</p>
    {:else}
      <div class="panel flush" style="margin:0"><table class="tbl compact">
        <thead><tr><th>{t('asv.link.colLink')}</th><th>{t('asv.link.colCreated')}</th><th>{t('asv.link.colExpires')}</th><th>{t('asv.link.colUsed')}</th><th>{t('asv.link.colMode')}</th><th>{t('asv.link.colNets')}</th><th>{t('asv.link.colGroup')}</th><th></th></tr></thead>
        <tbody>
          {#each links as link (link.id)}
            <tr>
              <td class="mono">{link.label}</td>
              <td>{fmtDateTime(link.created_at)}</td>
              <td>{expiresText(link)}</td>
              <td>{usedText(link)}{#each link.used as u (u.at + u.host)}<div class="dim">{u.host} · {fmtDateTime(u.at)}</div>{/each}</td>
              <td>{t(link.auto_approve ? 'asv.link.modeAutoShort' : 'asv.link.modeManualShort')}</td>
              <td class="mono">{link.networks.length > 0 ? link.networks.join(', ') : t('asv.link.netsAny')}</td>
              <td>{link.group_name ?? '—'}</td>
              <td class="r"><button class="btn sm" type="button" disabled={revoking[link.id]} onclick={() => void revoke(link)}>{t('asv.link.revoke')}</button></td>
            </tr>
          {/each}
        </tbody>
      </table></div>
    {/if}
  </Fold>
</div>
