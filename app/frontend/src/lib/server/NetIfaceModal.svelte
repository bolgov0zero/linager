<script lang="ts">
  import { untrack } from 'svelte';
  import { ApiError, serverApi2, type FieldError, type NetChange, type NetInterface } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { errorText } from '../errors';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import { changeFromForm, formFromIface, withChange, type NetForm } from '../net-draft';

  // The form of one interface: the change goes to the draft (nothing is applied yet); the draft is checked while typing.
  let { serverId, iface, draft, onsaved, onclose }: { serverId: string; iface: NetInterface; draft: NetChange[]; onsaved: (changes: NetChange[]) => void; onclose: () => void } = $props();

  const FIELDS: Record<string, string[]> = { mode: ['mode'], address: ['address'], gateway: ['gateway'], mtu: ['mtu'], state: ['state', 'up'] };
  const KNOWN = Object.values(FIELDS).flat();
  const form = $state<NetForm>(untrack(() => formFromIface(iface, draft.find((c) => c.iface === iface.name))));
  let errors = $state<FieldError[]>([]);
  let checking = $state(false);
  let saving = $state(false);
  let error = $state('');
  let seq = 0;

  // live validation, debounced
  $effect(() => {
    const changes = withChange(draft, changeFromForm({ ...form }));
    const my = ++seq;
    checking = true;
    const timer = setTimeout(async () => {
      try {
        const res = await serverApi2.networkValidate(serverId, { changes });
        if (my === seq) errors = res.errors;
      } catch {
        // saving reports its own error
      } finally {
        if (my === seq) checking = false;
      }
    }, 400);
    return () => clearTimeout(timer);
  });

  const fieldError = (name: string): string | null => errors.find((e) => e.iface === iface.name && (FIELDS[name] ?? [name]).includes(e.field))?.message ?? null;
  const own = $derived(errors.filter((e) => e.iface === iface.name && !KNOWN.includes(e.field)));
  const hasErrors = $derived(errors.length > 0);
  const modes = $derived([{ v: 'static', t: t('net.option.static') }, { v: 'dhcp', t: t('net.option.dhcp') }]);
  const states = $derived([{ v: 'UP', t: t('net.option.up') }, { v: 'DOWN', t: t('net.option.down') }]);

  async function save(): Promise<void> {
    if (saving || checking || hasErrors) return;
    const changes = withChange(draft, changeFromForm({ ...form }));
    saving = true;
    error = '';
    try {
      await serverApi2.networkSaveDraft(serverId, { changes });
      onsaved(changes);
    } catch (e) {
      error = e instanceof ApiError && e.status === 422 ? t('net.error.invalid') : errorText(e);
    } finally {
      saving = false;
    }
  }
</script>

<Modal title="{t('net.edit')} · {iface.name}" {onclose}>
  <div class="two">
    <div class="fl"><span>{t('net.form.mode')}</span><CustomSelect value={form.mode} options={modes} onchange={(v) => (form.mode = v as 'static' | 'dhcp')} />{#if fieldError('mode')}<span class="err-t">{fieldError('mode')}</span>{/if}</div>
    <div class="fl"><span>{t('net.form.state')}</span><CustomSelect value={form.state} options={states} onchange={(v) => (form.state = v as 'UP' | 'DOWN')} />{#if fieldError('state')}<span class="err-t">{fieldError('state')}</span>{/if}</div>
  </div>
  <label class="fl"><span>{t('net.form.address')}</span><span class="field"><input class="mono" autocomplete="off" spellcheck="false" placeholder={t('net.form.addressHint')} disabled={form.mode === 'dhcp'} bind:value={form.address} /></span>{#if fieldError('address')}<span class="err-t">{fieldError('address')}</span>{/if}</label>
  <div class="two">
    <label class="fl"><span>{t('net.form.gateway')}</span><span class="field"><input class="mono" autocomplete="off" spellcheck="false" disabled={form.mode === 'dhcp'} bind:value={form.gateway} /></span>{#if fieldError('gateway')}<span class="err-t">{fieldError('gateway')}</span>{/if}</label>
    <label class="fl"><span>{t('net.form.mtu')} <span class="dim">({t('net.form.mtuHint')})</span></span><span class="field"><input class="mono" type="number" min="68" max="9000" step="1" bind:value={form.mtu} /></span>{#if fieldError('mtu')}<span class="err-t">{fieldError('mtu')}</span>{/if}</label>
  </div>
  {#each own as e (e.field + e.message)}<div class="err">{e.message}</div>{/each}
  {#if error}<div class="err" role="alert">{error}</div>{/if}
  {#if checking}<p class="dim">{t('net.checking')}</p>{/if}
  {#snippet foot()}
    <span class="spacer"></span>
    <button class="btn" type="button" disabled={saving} onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" disabled={saving || checking || hasErrors} onclick={save}>{t('net.draft.save')}</button>
  {/snippet}
</Modal>
