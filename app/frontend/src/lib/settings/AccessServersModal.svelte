<script lang="ts">
  import { untrack } from 'svelte';
  import type { Group, ServerCard } from '../api';
  import { chainOf, flatten, subtreeIds, subtreesOf, type FolderNode } from '../folders';
  import Modal from '../Modal.svelte';
  import SearchField from '../SearchField.svelte';
  import { t } from '../i18n.svelte';

  // The servers an operator or an auditor sees: single servers and whole folders. A folder covers everything under it: its servers and
  // its subfolders with theirs, also the ones that join later; those are then included and cannot be chosen one by one.
  let { servers, groups, selectedServers, selectedGroups, onsave, onclose }: { servers: ServerCard[]; groups: Group[]; selectedServers: string[]; selectedGroups: number[]; onsave: (servers: string[], groups: number[]) => void; onclose: () => void } = $props();

  let pickedServers = $state<string[]>(untrack(() => [...selectedServers]));
  let pickedGroups = $state<number[]>(untrack(() => [...selectedGroups]));
  let search = $state('');

  const q = $derived(search.trim().toLowerCase());
  const matches = (s: ServerCard): boolean => !q || s.name.toLowerCase().includes(q) || s.address.toLowerCase().includes(q);
  const covered = $derived(subtreesOf(groups, pickedGroups));
  const viaGroup = (s: ServerCard): boolean => s.group_id != null && covered.has(s.group_id);
  const nodes = $derived(flatten(groups));
  // a folder is covered by itself or by a folder above it
  const coveredBy = (n: FolderNode): boolean => chainOf(groups, n.id).slice(1).some((id) => pickedGroups.includes(id));
  const under = (id: number): number => { const ids = subtreeIds(groups, id); return servers.filter((s) => s.group_id != null && ids.has(s.group_id)).length; };
  const shownFolder = (n: FolderNode): boolean => !q || servers.some((s) => s.group_id != null && subtreeIds(groups, n.id).has(s.group_id) && matches(s));
  const blocks = $derived.by(() => {
    const list = nodes.filter(shownFolder).map((n) => ({ node: n as FolderNode | null, servers: servers.filter((s) => s.group_id === n.id && matches(s)) }));
    const rest = servers.filter((s) => s.group_id == null && matches(s));
    if (rest.length > 0 || list.length === 0) list.push({ node: null, servers: rest });
    return list;
  });
  const total = $derived(new Set([...pickedServers, ...servers.filter(viaGroup).map((s) => s.id)]).size);

  const toggleGroup = (id: number): void => {
    pickedGroups = pickedGroups.includes(id) ? pickedGroups.filter((g) => g !== id) : [...pickedGroups, id];
  };
  const toggleServer = (s: ServerCard): void => {
    if (viaGroup(s)) return;
    pickedServers = pickedServers.includes(s.id) ? pickedServers.filter((x) => x !== s.id) : [...pickedServers, s.id];
  };
</script>

<Modal title={t('st.acc.servers.title')} {onclose}>
  <p class="dim">{t('st.acc.servers.hint')}</p>
  {#if servers.length > 8}<SearchField placeholder={t('st.acc.servers.search')} bind:value={search} spellcheck={false} />{/if}
  <div class="acc-blocks">
    {#each blocks as b (b.node?.id ?? 'none')}
      <section>
        {#if b.node}
          {@const n = b.node}
          <label class="ck acc-gh" class:dis={coveredBy(n)} style="padding-left:{n.depth * 16}px"><input type="checkbox" checked={pickedGroups.includes(n.id) || coveredBy(n)} disabled={coveredBy(n)} onchange={() => toggleGroup(n.id)} /><b>{n.name}</b><span class="dim">{t('st.acc.servers.groupNote', { n: under(n.id) })}</span></label>
        {:else}<div class="acc-gh"><b>{t('st.acc.servers.ungrouped')}</b></div>{/if}
        {#each b.servers as s (s.id)}
          <label class="ck acc-sr" class:dis={viaGroup(s)} style="padding-left:{(b.node ? b.node.depth + 1 : 0) * 16 + 14}px"><input type="checkbox" checked={viaGroup(s) || pickedServers.includes(s.id)} disabled={viaGroup(s)} onchange={() => toggleServer(s)} /><span>{s.name}</span><span class="dim mono">{s.address}</span></label>
        {:else}{#if b.node && !q && under(b.node.id) === 0}<p class="dim acc-sr" style="padding-left:{(b.node.depth + 1) * 16 + 14}px">{t('st.acc.servers.empty')}</p>{/if}{/each}
      </section>
    {:else}<p class="dim">{t('st.acc.servers.none')}</p>{/each}
  </div>
  {#snippet foot()}
    <span class="dim">{t('st.acc.servers.total', { n: total })}</span>
    <span class="spacer"></span>
    <button class="btn" type="button" onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" onclick={() => onsave(pickedServers, pickedGroups)}>{t('st.acc.apply')}</button>
  {/snippet}
</Modal>
