<script lang="ts">
  import { tick } from 'svelte';
  import { answer, dialog, prompter, promptAnswer } from './dialogs.svelte';
  import { t } from './i18n.svelte';

  let pv = $state('');
  let perr = $state('');
  let pinput = $state<HTMLInputElement | null>(null);
  $effect(() => {
    if (prompter.open) {
      pv = prompter.spec?.value ?? '';
      perr = '';
      void tick().then(() => {
        pinput?.focus();
        pinput?.select();
      });
    }
  });
  function psubmit(): void {
    const e = prompter.spec?.check?.(pv.trim()) ?? '';
    if (e) {
      perr = e;
      return;
    }
    promptAnswer(pv.trim());
  }
</script>

<svelte:window onkeydown={(e) => { if (e.key !== 'Escape') return; if (dialog.open) answer(false); else if (prompter.open) promptAnswer(null); }} />

{#if dialog.open && dialog.ask}
  <div class="dlg-scrim" role="presentation" onclick={(e) => e.target === e.currentTarget && answer(false)}>
    <div class="dlg" role="alertdialog" aria-modal="true" aria-label={dialog.ask.title}>
      <h3>{dialog.ask.title}</h3>
      <p>{dialog.ask.text}</p>
      <div class="row end">
        <button class="btn" type="button" onclick={() => answer(false)}>{dialog.ask.cancel ?? t('common.cancel')}</button>
        <button class="btn {dialog.ask.danger ? 'danger' : 'primary'}" type="button" onclick={() => answer(true)}>{dialog.ask.ok}</button>
      </div>
    </div>
  </div>
{/if}

{#if prompter.open && prompter.spec}
  <div class="dlg-scrim" role="presentation" onclick={(e) => e.target === e.currentTarget && promptAnswer(null)}>
    <form class="dlg" aria-label={prompter.spec.title} onsubmit={(e) => { e.preventDefault(); psubmit(); }}>
      <h3>{prompter.spec.title}</h3>
      <label class="fl"><span>{prompter.spec.label}</span><span class="field"><input bind:this={pinput} bind:value={pv} oninput={() => (perr = '')} spellcheck="false" autocomplete="off" /></span></label>
      {#if perr}<div class="err" role="alert">{perr}</div>{/if}
      <div class="row end">
        <button class="btn" type="button" onclick={() => promptAnswer(null)}>{t('common.cancel')}</button>
        <button class="btn primary" type="submit">{prompter.spec.ok}</button>
      </div>
    </form>
  </div>
{/if}
