<script lang="ts">
  import { untrack } from 'svelte';
  import type { TagRow } from '../api';
  import Modal from '../Modal.svelte';
  import SearchField from '../SearchField.svelte';
  import { t } from '../i18n.svelte';
  import TagChip from '../TagChip.svelte';

  // The tags given to an operator or an auditor: every server that has the tag is visible, also a server that gets the tag
  // later. This adds to the single servers and the groups: a server is visible when any of them covers it.
  let { tags, selected, onsave, onclose }: { tags: TagRow[]; selected: number[]; onsave: (tags: number[]) => void; onclose: () => void } = $props();

  let picked = $state<number[]>(untrack(() => [...selected]));
  let search = $state('');

  const q = $derived(search.trim().toLowerCase());
  const shown = $derived(tags.filter((x) => !q || x.name.toLowerCase().includes(q)));
  const toggle = (id: number): void => {
    picked = picked.includes(id) ? picked.filter((x) => x !== id) : [...picked, id];
  };
</script>

<Modal title={t('st.acc.tags.title')} {onclose}>
  <p class="dim">{t('st.acc.tags.hint')}</p>
  {#if tags.length > 8}<SearchField placeholder={t('st.acc.tags.search')} bind:value={search} spellcheck={false} />{/if}
  <div class="acc-blocks">
    {#each shown as tag (tag.id)}
      <label class="ck acc-sr"><input type="checkbox" checked={picked.includes(tag.id)} onchange={() => toggle(tag.id)} /><TagChip name={tag.name} color={tag.color} /><span class="dim">{t('st.acc.tags.servers', { n: tag.servers })}</span>{#if tag.servers === 0}<span class="dim">· {t('st.acc.tags.noServers')}</span>{/if}</label>
    {:else}<p class="dim">{t('st.acc.tags.none')}</p>{/each}
  </div>
  {#snippet foot()}
    <span class="dim">{t('st.acc.tags.total', { n: picked.length })}</span>
    <span class="spacer"></span>
    <button class="btn" type="button" onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" onclick={() => onsave(picked)}>{t('st.acc.apply')}</button>
  {/snippet}
</Modal>
