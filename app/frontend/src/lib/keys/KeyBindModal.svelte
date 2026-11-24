<script lang="ts">
  import { untrack } from 'svelte';
  import Modal from '../Modal.svelte';
  import SearchField from '../SearchField.svelte';
  import { t } from '../i18n.svelte';
  import { chainOf, flatten, subtreesOf } from '../folders';
  import type { Directory } from './store';

  // Where a key is bound: folders and servers. A folder covers all its servers and the folders under it, also the future ones. Nothing marked: all servers.
  let { dir, servers, groups, onapply, onclose }: { dir: Directory; servers: string[]; groups: number[]; onapply: (servers: string[], groups: number[]) => void; onclose: () => void } = $props();

  let pickedS = $state<string[]>(untrack(() => [...servers]));
  let pickedG = $state<number[]>(untrack(() => [...groups]));
  let q = $state('');

  const coveredFolders = $derived(subtreesOf(dir.groups, pickedG));
  const covered = (id: string): boolean => {
    const g = dir.servers.find((s) => s.id === id)?.group_id;
    return g != null && coveredFolders.has(g);
  };
  const nodes = $derived(flatten(dir.groups));
  const viaParent = (id: number): boolean => chainOf(dir.groups, id).slice(1).some((x) => pickedG.includes(x));
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
    <div class="bind-list">{#each nodes as g (g.id)}<label class="ck" style="padding-left:{g.depth * 16}px"><input type="checkbox" checked={pickedG.includes(g.id) || viaParent(g.id)} disabled={viaParent(g.id)} onchange={() => toggleG(g.id)} />{g.name}</label>{/each}</div>
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
