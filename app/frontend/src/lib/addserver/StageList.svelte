<script lang="ts" module>
  export interface Stage {
    id: string;
    state: 'pending' | 'running' | 'done' | 'warn' | 'failed';
    /** Already translated name of the stage. */
    name: string;
    /** The grey line under the name; red on a failed stage, amber on a warning. */
    note?: string;
    /** Unix milliseconds on the server clock. */
    startedMs?: number;
    finishedMs?: number;
    /** 0..1: draws a progress bar under the note. */
    progress?: number;
  }
</script>

<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from '../Icon.svelte';
  import { fmtDuration } from '../settings/upd-format';

  // The list of stages of a long job (✓ ✗ −, a spinner for the running one, the time of every stage and a grey note). The
  // times are on the server clock; nowMs is the server clock at the moment the data was made, so the running timer does
  // not depend on the clock of this window.
  let { stages, nowMs = 0, label }: { stages: Stage[]; nowMs?: number; label?: string } = $props();

  let skew = $state(0);
  let tick = $state(Date.now());
  $effect(() => {
    skew = nowMs > 0 ? Date.now() - nowMs : 0;
  });
  const anyRunning = $derived(stages.some((s) => s.state === 'running'));
  onMount(() => {
    const timer = setInterval(() => {
      if (anyRunning) tick = Date.now();
    }, 250);
    return () => clearInterval(timer);
  });
  const serverNow = $derived(tick - skew);
  function elapsed(stage: Stage): string {
    if (!stage.startedMs) return '';
    const end = stage.state === 'running' ? serverNow : stage.finishedMs;
    return end ? fmtDuration(end - stage.startedMs) : '';
  }
</script>

<ol class="ustep stages" aria-label={label}>
  {#each stages as stage (stage.id)}
    {@const time = elapsed(stage)}
    <li class="s-{stage.state}">
      <span class="ico" aria-hidden="true">{#if stage.state === 'done'}<Icon name="check" size={12} />{:else if stage.state === 'running'}<span class="spin"><i></i><i></i></span>{:else if stage.state === 'failed'}<Icon name="x" size={12} />{:else if stage.state === 'warn'}<Icon name="triangle-alert" size={12} />{:else}<Icon name="minus" size={12} />{/if}</span>
      <span>{stage.name}</span>
      {#if time}<span class="dim mono">{time}</span>{/if}
      {#if stage.note}<span class="dim" class:crit={stage.state === 'failed'}>{stage.note}</span>{/if}
      {#if stage.progress !== undefined}<span class="bar lv-ok sbar"><i style="width:{Math.round(Math.min(Math.max(stage.progress, 0), 1) * 100)}%"></i></span>{/if}
    </li>
  {/each}
</ol>
