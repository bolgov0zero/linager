<script lang="ts">
  import { untrack } from 'svelte';
  import { serverApi2 } from '../api';
  import { errorText } from '../errors';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import { checkDns, DNS_MAX_SERVERS } from '../net-dns';

  let { serverId, nameservers, search, onclose, onapplied }: { serverId: string; nameservers: string[]; search: string[]; onclose: () => void; onapplied: () => void } = $props();

  let rows = $state<string[]>(untrack(() => (nameservers.slice(0, DNS_MAX_SERVERS).length > 0 ? nameservers.slice(0, DNS_MAX_SERVERS) : [''])));
  let searchText = $state(untrack(() => search.join(' ')));
  let busy = $state(false);
  let error = $state('');
  const check = $derived(checkDns(rows, searchText));
  const same = $derived(check.nameservers.join(' ') === nameservers.join(' ') && check.domains.join(' ') === search.join(' '));

  const addRow = (): void => {
    if (rows.length < DNS_MAX_SERVERS) rows = [...rows, ''];
  };
  const removeRow = (i: number): void => {
    rows = rows.length > 1 ? rows.filter((_, j) => j !== i) : [''];
  };
  async function submit(): Promise<void> {
    if (busy || !check.ok || same) return;
    busy = true;
    error = '';
    try {
      await serverApi2.networkApplyDns(serverId, check.nameservers, check.domains);
      onapplied();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t('net.dns.edit.title')} {onclose}>
  <div class="fl"><span>{t('net.dns.servers')} <span class="dim">({t('net.dns.edit.limit', { n: DNS_MAX_SERVERS })})</span></span>
    {#each rows as _, i (i)}
      <div class="row">
        <span class="field grow"><input class="mono" autocomplete="off" spellcheck="false" aria-label={t('net.dns.servers')} bind:value={rows[i]} /></span>
        <button class="btn icon sm" type="button" aria-label={t('common.clear')} onclick={() => removeRow(i)}><Icon name="x" size={14} /></button>
      </div>
      {#if check.rows[i]}<span class="err-t">{t(check.rows[i]!)}</span>{/if}
    {/each}
    {#if check.list}<span class="err-t">{t(check.list)}</span>{/if}
    {#if rows.length < DNS_MAX_SERVERS}<button class="btn sm" type="button" style="justify-self:start" onclick={addRow}>{t('net.dns.edit.add')}</button>{/if}
  </div>
  <label class="fl"><span>{t('net.dns.search')}</span><span class="field"><input class="mono" autocomplete="off" spellcheck="false" bind:value={searchText} /></span>{#if check.search}<span class="err-t">{t(check.search)}</span>{/if}</label>
  <p class="dim">{t('net.dns.edit.note')}</p>
  {#if error}<div class="err" role="alert">{error}</div>{/if}
  {#snippet foot()}
    <span class="spacer"></span>
    <button class="btn" type="button" disabled={busy} onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" disabled={busy || !check.ok || same} onclick={submit}>{t('net.dns.edit.apply')}</button>
  {/snippet}
</Modal>
