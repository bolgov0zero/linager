<script lang="ts">
  import type { AgentUpdateRow, RolloutMode, UpdatesInfo } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { shortOs } from '../status';
  import { t } from '../i18n.svelte';
  import UpdAgentSteps from './UpdAgentSteps.svelte';
  import { APPLICABLE } from './upd-format';

  let { info, onrun }: { info: UpdatesInfo; /** Resolves true when the request was accepted. */ onrun: (ids: string[] | 'all', mode: RolloutMode, stopOnError: boolean) => Promise<boolean> } = $props();

  const TOTAL_STEPS = 8;

  let mode = $state<RolloutMode>('first_then_rest');
  let stopOnError = $state(true);
  /** Checkbox state by agent id. Kept apart from `info`, so refreshes never reset it. */
  let picked = $state<Record<string, boolean>>({});
  let submitting = $state(false);
  /** Agents whose list of stages is open. Kept apart from `info`, so refreshes never close it. */
  let opened = $state<Record<string, boolean>>({});

  const applicable = $derived(info.agents.filter((a) => APPLICABLE.has(a.status)));
  const selectedIds = $derived(applicable.filter((a) => picked[a.id]).map((a) => a.id));
  const rolloutRunning = $derived(info.rollout?.state === 'running');
  const canAct = $derived(!rolloutRunning && !submitting);
  const allSelected = $derived(applicable.length > 0 && selectedIds.length === applicable.length);
  const needServer = $derived(Boolean(info.server.available) && info.server.available !== info.server.current);
  const modeOptions = $derived([{ v: 'all', t: t('st.upd.mode.all') }, { v: 'one_by_one', t: t('st.upd.mode.one') }, { v: 'first_then_rest', t: t('st.upd.mode.first') }]);

  async function run(ids: string[] | 'all', clear: string[] = []): Promise<void> {
    submitting = true;
    const ok = await onrun(ids, mode, stopOnError);
    submitting = false;
    if (ok && clear.length > 0) {
      const next = { ...picked };
      for (const id of clear) next[id] = false;
      picked = next;
    }
  }
  function toggleAll(on: boolean): void {
    const next = { ...picked };
    for (const a of applicable) next[a.id] = on;
    picked = next;
  }
  const stepName = (a: AgentUpdateRow): string => (a.step_id ? t(`st.upd.agstep.${a.step_id}`) : '');
</script>

<div class="upd-agents">
  <p class="dim">{t('st.upd.agents.intro')} <b class="mono">{info.agents_target}</b>. {#if needServer}{t('st.upd.agents.needServer', { version: info.server.available ?? '' })}{/if}</p>
  {#if rolloutRunning}<p class="warnbox warn">{t('st.upd.agents.rolloutRunning')}</p>{/if}

  <div class="row" style="flex-wrap:wrap">
    <span class="dim">{t('st.upd.agents.order')}</span>
    <CustomSelect small value={mode} options={modeOptions} onchange={(v) => (canAct ? (mode = v as RolloutMode) : undefined)} />
    <label class="ck"><input type="checkbox" bind:checked={stopOnError} />{t('st.upd.agents.stopOnError')}</label>
    <span class="upd-act">
      <button class="btn sm" type="button" disabled={!canAct || selectedIds.length === 0} onclick={() => run(selectedIds, selectedIds)}>{t('st.upd.agents.runSelected', { count: selectedIds.length })}</button>
      <button class="btn sm primary" type="button" disabled={!canAct || applicable.length === 0} onclick={() => run('all')}>{t('st.upd.agents.runAll', { count: applicable.length })}</button>
    </span>
  </div>

  <div class="panel flush" style="margin:0"><table class="tbl compact uag">
    <thead><tr>
      <th style="width:34px"><input type="checkbox" aria-label={t('st.upd.agents.selectAll')} checked={allSelected} disabled={!canAct || applicable.length === 0} onchange={(e) => toggleAll(e.currentTarget.checked)} /></th>
      <th>{t('st.upd.agents.colServer')}</th><th>{t('st.upd.agents.colVersion')}</th><th>{t('st.upd.agents.colStatus')}</th><th>{t('st.upd.agents.colProgress')}</th><th></th>
    </tr></thead>
    <tbody>
      {#each info.agents as agent (agent.id)}
        {@const isApplicable = APPLICABLE.has(agent.status)}
        <tr>
          <td>{#if isApplicable}<input type="checkbox" aria-label={t('st.upd.agents.select', { name: agent.name })} bind:checked={picked[agent.id]} disabled={!canAct} />{/if}</td>
          <td class="srvc"><div class="cut"><b>{agent.name}</b></div><div class="dim cut" title="{agent.os} · {agent.address}">{shortOs(agent.os)} · {agent.address}</div><div class="nm-ver mono">{agent.version}</div></td>
          <td class="mono">{agent.version}</td>
          <td class="stc">
            {#if agent.status === 'current' && agent.manual}<span class="badge b-warn">{t('st.upd.ag.manual', { version: agent.target ?? '' })}</span>
            {:else if agent.status === 'current'}<span class="badge b-ok">{t('st.upd.ag.current')}</span>
            {:else if agent.status === 'available'}<span class="badge b-warn">{t('st.upd.ag.available', { version: agent.target ?? info.server.available ?? '' })}</span>
            {:else if agent.status === 'unsupported'}<span class="badge b-crit">{t('st.upd.ag.unsupported')}</span>
            {:else if agent.status === 'updating'}<span class="badge b-warn">{t('st.upd.ag.updating')}</span>
            {:else if agent.status === 'queued'}<span class="badge b-mute">{t('st.upd.ag.queued')}</span>{#if !agent.online}<span class="dim" title={t('st.upd.ag.waiting')}> {t('st.upd.ag.waiting')}</span>{/if}
            {:else if agent.status === 'rollback'}<span class="badge b-crit">{t('st.upd.ag.rollback')}</span>
            {:else}<span class="badge b-crit">{t('st.upd.ag.failed')}</span>{/if}
          </td>
          <td class="pgc">
            {#if agent.status === 'updating'}
              <div class="prow" title="{t('st.upd.ag.step', { n: agent.step ?? 0, total: agent.steps_total || TOTAL_STEPS, name: stepName(agent) })}{agent.message ? ` · ${agent.message}` : ''}">
                <div class="upb" aria-hidden="true">{#each Array.from({ length: agent.steps_total || TOTAL_STEPS }) as _, i (i)}<i class:on={i < (agent.step ?? 0)}></i>{/each}</div>
                <span class="dim cut">{agent.step ?? 0}/{agent.steps_total || TOTAL_STEPS} · {stepName(agent)}</span>
              </div>
            {:else if agent.status === 'rollback' || agent.status === 'failed'}
              <div class="prow">
                <span class="critt cut" title={agent.message ?? ''}>{agent.message ?? ''}</span>
                {#if agent.steps?.length}<button class="linkbtn" type="button" aria-expanded={Boolean(opened[agent.id])} onclick={() => (opened = { ...opened, [agent.id]: !opened[agent.id] })}>{opened[agent.id] ? t('st.upd.agstep.hide') : t('st.upd.agstep.show')}</button>{/if}
              </div>
              {#if opened[agent.id] && agent.steps?.length}<div class="uag-steps"><UpdAgentSteps steps={agent.steps} nowMs={agent.now_ms} rollbackAt={agent.rollback_at} /></div>{/if}
            {:else if agent.status === 'queued' && agent.message}<span class="dim cut" title={agent.message}>{agent.message}</span>
            {:else}<span class="dim">—</span>{/if}
          </td>
          <td class="r">{#if isApplicable}<button class="btn sm" type="button" disabled={!canAct} onclick={() => run([agent.id])}>{t('st.upd.agents.update')}</button>{/if}</td>
        </tr>
      {:else}
        <tr><td colspan="6" class="dim">{t('st.upd.agents.empty')}</td></tr>
      {/each}
    </tbody>
  </table></div>
</div>
