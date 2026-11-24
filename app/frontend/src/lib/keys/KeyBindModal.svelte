<script lang="ts">
  import { untrack } from 'svelte';
  import Modal from '../Modal.svelte';
  import SearchField from '../SearchField.svelte';
  import { t } from '../i18n.svelte';
  import type { Directory } from './store';

  // Where a key is bound: groups and servers. A group covers all its servers, also the future ones. Nothing marked: all servers.
  let { dir, servers, groups, onapply, onclose }: { dir: Directory; servers: string[]; groups: number[]; onapply: (servers: string[], groups: number[]) => void; onclose: () => void } = $props();

  let pickedS = $state<string[]>(untrack(() => [...servers]));
  let pickedG = $state<number[]>(untrack(() => [...groups]));
  let q = $state('');

  const covered = (id: string): boolean => {
    const g = dir.servers.find((s) => s.id === id)?.group_id;
    return g != null && pickedG.includes(g);
  };
  const shown = $derived(dir.servers.filter((s) => !q.trim() || s.name.toLowerCase().includes(q.trim().toLowerCase())).sort((a, b) => a.name.localeCompare(b.name)));
  const total = $derived(dir.servers.filter((s) => pickedS.includes(s.id) || covered(s.id)).length);
  const toggleS = (id: string): void => {
    pickedS = pickedS.includes(id) ? pickedS.filter((x) => x !== id) : [...pickedS, id];
  };
  const toggleG = (id: number): void => {
    pickedG = pickedG.includes(id) ? pickedG.filter((x) => x !== id) : [...pickedG, id];
  };
</script>

<Modal title={t('keys.bindModal.title')} {onclose}>
  <p class="dim">{t('keys.bindModal.hint')}</p>
  {#if dir.groups.length}
    <div class="chk-grid">{#each dir.groups as g (g.id)}<label class="ck"><input type="checkbox" checked={pickedG.includes(g.id)} onchange={() => toggleG(g.id)} />{g.name}</label>{/each}</div>
  {/if}
  <SearchField placeholder={t('pult.search')} bind:value={q} spellcheck={false} />
  <div class="bind-list">
    {#each shown as s (s.id)}
      <label class="ck"><input type="checkbox" checked={pickedS.includes(s.id) || covered(s.id)} disabled={covered(s.id)} onchange={() => toggleS(s.id)} />{s.name}</label>
    {/each}
  </div>
  <p class="dim">{pickedS.length + pickedG.length === 0 ? t('keys.bindModal.none') : t('keys.bindModal.total', { n: total })}</p>
  {#snippet foot()}
    <span class="spacer"></span>
    <button class="btn" type="button" onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" onclick={() => onapply(pickedS, pickedG)}>{t('keys.edit.done')}</button>
  {/snippet}
</Modal>
