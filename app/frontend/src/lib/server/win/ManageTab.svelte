<script lang="ts">
  import { assistantApi, ApiError, desktopApi, type AssistSas, type AssistSession, type DesktopState, type ServerDetail } from '../../api';
  import AssistantQuality from '../../assistant/AssistantQuality.svelte';
  import { t } from '../../i18n.svelte';
  import Modal from '../../Modal.svelte';
  import { native } from '../../native';
  import { showToast } from '../../toast';

  // Windows: the picture settings of the Assistant, the service of the remote desktop (turn it on or off), the sessions of the
  // computer and the policy that lets the agent send Ctrl+Alt+Del.
  let { s, canDesktop = true, nativeWindows = false, onchanged }: { s: ServerDetail; canDesktop?: boolean; nativeWindows?: boolean; /** The state of the remote desktop changed: the page reads the server again. */ onchanged?: () => void } = $props();

  const serverId = $derived(s.id);
  const online = $derived(s.online);
  const err = (e: unknown): string => (e instanceof ApiError && e.code ? e.code : t('common.genericError'));

  // ---- remote desktop (RDP) ----
  let rdp = $state<DesktopState | null>(null);
  let rdpError = $state('');
  let rdpBusy = $state(false);
  let confirmOff = $state(false);
  let qualityOpen = $state(false);

  async function loadRdp(): Promise<void> {
    try {
      rdp = await desktopApi.state(serverId);
      rdpError = '';
    } catch (e) {
      rdp = null;
      rdpError = err(e);
    }
  }
  $effect(() => {
    void serverId;
    if (canDesktop && online) void loadRdp();
  });
  async function rdpEnable(): Promise<void> {
    rdpBusy = true;
    try {
      await desktopApi.enable(serverId);
      showToast(t('desk.enabled'));
      await loadRdp();
      onchanged?.();
    } catch (e) {
      showToast(err(e));
    } finally {
      rdpBusy = false;
    }
  }
  async function rdpDisable(): Promise<void> {
    rdpBusy = true;
    try {
      await desktopApi.disable(serverId);
      confirmOff = false;
      showToast(t('manage.rdpDisabled'));
      await loadRdp();
      onchanged?.();
    } catch (e) {
      confirmOff = false;
      showToast(err(e));
    } finally {
      rdpBusy = false;
    }
  }

  // ---- Windows sessions ----
  let sessions = $state<AssistSession[] | null>(null);
  let sessionsError = $state('');
  /** The Ctrl+Alt+Del policy of Windows; null while unknown or when the agent does not report it. */
  let sas = $state<AssistSas | null>(null);
  let sasBusy = $state(false);
  /** The dialog of the Ctrl+Alt+Del policy: the value it would set, null when closed. */
  let confirmSas = $state<boolean | null>(null);

  async function loadSessions(): Promise<void> {
    try {
      const res = await assistantApi.sessions(serverId);
      sessions = res.sessions;
      if (res.sas) sas = res.sas;
      sessionsError = '';
    } catch (e) {
      sessionsError = e instanceof ApiError && e.status === 501 ? t('manage.noModule') : t('common.genericError');
    }
  }
  $effect(() => {
    void serverId;
    if (!online) return;
    void loadSessions();
    const timer = setInterval(() => void loadSessions(), 15000);
    return () => clearInterval(timer);
  });
  async function setSas(enable: boolean): Promise<void> {
    sasBusy = true;
    try {
      sas = await assistantApi.setSas(serverId, enable);
      showToast(t(sas.enabled ? 'manage.sasAllowed' : 'manage.sasDenied'));
    } catch (e) {
      showToast(e instanceof ApiError && e.status === 501 ? t('manage.noModule') : err(e));
    } finally {
      confirmSas = null;
      sasBusy = false;
    }
  }

  const rdpSessions = $derived((sessions ?? []).filter((x) => x.type === 'rdp'));
  const rdpActive = $derived(rdpSessions.filter((x) => x.state === 'active').length);
  const rdpOn = $derived(!!rdp && rdp.enabled && rdp.listening);
  function stateText(x: AssistSession): string {
    const key = `manage.state.${x.state}`;
    const text = t(key);
    return text === key ? x.state : text;
  }
  const openSession = (): void => void native.openWindow('asst', serverId, s.name, nativeWindows);
</script>

<div class="manage">
  <p class="dim">{t('manage.lead')}</p>
  <div class="mg-grid">
    <section class="panel"><h4 class="ph">{t('manage.picture')}</h4><p class="dim">{t('manage.presetsHint')}</p><div class="row" style="margin-top:10px"><button class="btn" type="button" disabled={!online} onclick={() => (qualityOpen = true)}>{t('assist.quality')}</button></div></section>

    <div class="mg-col">
      {#if canDesktop}
        <section class="panel">
          <h4 class="ph">{t('manage.rdp')}</h4>
          {#if !online}<p class="dim">{t('manage.offline')}</p>
          {:else if rdpError}<div class="err" role="alert">{rdpError}</div>
          {:else if !rdp}<p class="dim">{t('dash.loading')}</p>
          {:else if !rdp.supported}
            <span class="badge b-warn">{t('manage.rdpUnavailable')}</span>
            <p class="dim">{t('manage.rdpHome', { edition: rdp.edition })}</p>
          {:else}
            <div class="row"><span class="badge {rdpOn ? 'b-ok' : 'b-warn'}">{rdp.enabled ? t('manage.rdpOn') : t('manage.rdpOff')}</span></div>
            <dl class="facts one">
              <div><dt>{t('manage.rdpPort')}</dt><dd>{rdp.port}</dd></div>
              <div><dt>{t('manage.rdpService')}</dt><dd>{rdp.service || '—'}</dd></div>
              <div><dt>{t('manage.rdpListening')}</dt><dd>{rdp.listening ? t('win.yes') : t('win.no')}</dd></div>
              <div><dt>{t('manage.rdpSessions')}</dt><dd>{sessions ? t('manage.rdpSessionsValue', { n: rdpSessions.length, active: rdpActive }) : '—'}</dd></div>
            </dl>
            <div class="row">
              {#if rdp.enabled}<button class="btn danger" type="button" disabled={rdpBusy} onclick={() => (confirmOff = true)}>{t('manage.rdpDisable')}</button>
              {:else}<button class="btn primary" type="button" disabled={rdpBusy} onclick={() => void rdpEnable()}>{t('manage.rdpEnable')}</button>{/if}
            </div>
            <p class="dim">{rdp.enabled ? t('manage.rdpOnNote') : t('manage.rdpOffNote')}</p>
          {/if}
        </section>
      {/if}

      <section class="panel">
        <h4 class="ph">{t('manage.sessions')}</h4>
        {#if !online}<p class="dim">{t('manage.offline')}</p>
        {:else if sessionsError}<div class="err" role="alert">{sessionsError}</div>
        {:else if !sessions}<p class="dim">{t('dash.loading')}</p>
        {:else if sessions.length === 0}<p class="dim">{t('manage.noSessions')}</p>
        {:else}
          <div class="panel flush" style="margin:0"><table class="tbl compact">
            <thead><tr><th>{t('manage.col.id')}</th><th>{t('manage.col.user')}</th><th>{t('manage.col.type')}</th><th>{t('manage.col.state')}</th><th></th></tr></thead>
            <tbody>
              {#each sessions as x (x.id)}
                <tr>
                  <td>{x.id}</td>
                  <td>{x.user || t('assist.noUser')}</td>
                  <td>{t(`assist.type.${x.type}`)}{#if x.client}<div class="dim">{x.client}</div>{/if}</td>
                  <td class={x.state === 'active' ? 'okt' : 'warnt'}>{stateText(x)}</td>
                  <td class="r">{#if x.screen}<button class="btn sm" type="button" onclick={openSession}>{t('assist.name')}</button>{:else}<span class="dim">{t('assist.noScreen')}</span>{/if}</td>
                </tr>
              {/each}
            </tbody>
          </table></div>
        {/if}
      </section>

      {#if online && sas}
        <section class="panel">
          <h4 class="ph">{t('manage.sas')}</h4>
          <div class="row"><span class="badge {sas.enabled ? 'b-ok' : 'b-warn'}">{sas.enabled ? t('manage.sasOn') : t('manage.sasOff')}</span></div>
          <p class="dim">{t('manage.sasNote')}</p>
          <p class="dim">{t('manage.sasPolicy', { n: sas.policy })}</p>
          <div class="row">
            {#if sas.enabled}<button class="btn danger" type="button" disabled={sasBusy} onclick={() => (confirmSas = false)}>{t('manage.sasDeny')}</button>
            {:else}<button class="btn primary" type="button" disabled={sasBusy} onclick={() => (confirmSas = true)}>{t('manage.sasAllow')}</button>{/if}
          </div>
        </section>
      {/if}
    </div>
  </div>
</div>

{#if confirmSas !== null}
  <Modal title={confirmSas ? t('manage.sasAllowTitle') : t('manage.sasDenyTitle')} onclose={() => (confirmSas = null)}>
    <p>{confirmSas ? t('manage.sasAllowText') : t('manage.sasDenyText')}</p>
    <p class="dim">{t('manage.sasNote2')}</p>
    {#snippet foot()}
      <span class="spacer"></span>
      <button class="btn" type="button" onclick={() => (confirmSas = null)}>{t('win.cancel')}</button>
      <button class="btn danger" type="button" disabled={sasBusy} onclick={() => void setSas(confirmSas === true)}>{confirmSas ? t('manage.sasAllow') : t('manage.sasDeny')}</button>
    {/snippet}
  </Modal>
{/if}

{#if confirmOff}
  <Modal title={t('manage.rdpOffTitle')} onclose={() => (confirmOff = false)}>
    <p>{t('manage.rdpOffText')}</p>
    <p class="dim">{t('manage.rdpOffNote2')}</p>
    {#snippet foot()}
      <span class="spacer"></span>
      <button class="btn" type="button" onclick={() => (confirmOff = false)}>{t('win.cancel')}</button>
      <button class="btn danger" type="button" disabled={rdpBusy} onclick={() => void rdpDisable()}>{t('manage.rdpDisable')}</button>
    {/snippet}
  </Modal>
{/if}

{#if qualityOpen}<AssistantQuality {serverId} onclose={() => (qualityOpen = false)} />{/if}
