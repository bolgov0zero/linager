<script lang="ts">
  import { onMount } from 'svelte';
  import type { AgentUpdateStep } from '../api';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import { fmtDuration, fmtSize } from './upd-format';

  let { steps, nowMs = 0, rollbackAt = 0 }: { steps: AgentUpdateStep[]; /** The server clock at the moment the data was made; the times of the stages are on that clock. */ nowMs?: number; /** Unix seconds of the rollback of the agent; 0 when none is armed. */ rollbackAt?: number } = $props();

  /** How far the clock of this window is ahead of the server's, so running timers do not depend on either. */
  let skew = $state(0);
  let tick = $state(Date.now());
  $effect(() => {
    skew = nowMs > 0 ? Date.now() - nowMs : 0;
  });
  onMount(() => {
    const timer = setInterval(() => (tick = Date.now()), 1000);
    return () => clearInterval(timer);
  });
  const serverNow = $derived(tick - skew);

  function elapsed(step: AgentUpdateStep): string {
    if (!step.started_ms) return '';
    const end = step.state === 'running' ? serverNow : step.finished_ms;
    return end ? fmtDuration(end - step.started_ms) : '';
  }
  /** The grey line under a stage: the size of the download, the version that came back, a countdown, an error. */
  function note(step: AgentUpdateStep): string {
    if (step.state === 'failed' && step.detail) return step.detail;
    if (step.id === 'download' && step.size && step.size > 0) return step.state === 'running' ? t('st.upd.agstep.of', { done: fmtSize(step.bytes ?? 0), total: fmtSize(step.size) }) : fmtSize(step.size);
    if (step.id === 'reconnect' && step.detail) return t('st.upd.agstep.version', { version: step.detail });
    if (rollbackAt > 0 && (step.id === 'offline' || step.id === 'reconnect' || step.id === 'confirm') && step.state === 'running') {
      const left = Math.ceil(rollbackAt - serverNow / 1000);
      return left > 0 ? t('st.upd.agstep.rollbackIn', { n: left }) : t('st.upd.agstep.rollbackDue');
    }
    return '';
  }
</script>

<ol class="ustep" aria-label={t('st.upd.agstep.aria')}>
  {#each steps as step (step.id)}
    {@const text = note(step)}
    {@const time = elapsed(step)}
    <li class="s-{step.state}">
      <span class="ico" aria-hidden="true">{#if step.state === 'done'}<Icon name="check" size={12} />{:else if step.state === 'running'}<span class="spin"><i></i><i></i></span>{:else if step.state === 'failed'}<Icon name="x" size={12} />{:else}<Icon name="minus" size={12} />{/if}</span>
      <span>{t(`st.upd.agstep.${step.id}`)}</span>
      {#if time}<span class="dim mono">{time}</span>{/if}
      {#if text}<span class="dim" class:crit={step.state === 'failed'}>{text}</span>{/if}
    </li>
  {/each}
</ol>
