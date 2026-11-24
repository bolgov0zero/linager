<script lang="ts">
  import { untrack } from 'svelte';
  import type { Group, ServerCard } from '../api';
  import Modal from '../Modal.svelte';
  import SearchField from '../SearchField.svelte';
  import { t } from '../i18n.svelte';

  // The servers an operator or an auditor sees: single servers and whole groups (a group also covers the servers that
  // join it later, and its servers are then included and cannot be chosen one by one).
  let { servers, groups, selectedServers, selectedGroups, onsave, onclose }: { servers: ServerCard[]; groups: Group[]; selectedServers: string[]; selectedGroups: number[]; onsave: (servers: string[], groups: number[]) => void; onclose: () => void } = $props();

  let pickedServers = $state<string[]>(untrack(() => [...selectedServers]));
  let pickedGroups = $state<number[]>(untrack(() => [...selectedGroups]));
  let search = $state('');

  const q = $derived(search.trim().toLowerCase());
  const matches = (s: ServerCard): boolean => !q || s.name.toLowerCase().includes(q) || s.address.toLowerCase().includes(q);
  const viaGroup = (s: ServerCard): boolean => s.group_id != null && pickedGroups.includes(s.group_id);

  const blocks = $derived.by(() => {
    const list = groups.slice().sort((a, b) => a.name.localeCompare(b.name)).map((g) => ({ group: g as Group | null, servers: servers.filter((s) => s.group_id === g.id && matches(s)) }));
    const rest = servers.filter((s) => s.group_id == null && matches(s));
    if (rest.length > 0 || list.length === 0) list.push({ group: null, servers: rest });
    return list.filter((b) => b.servers.length > 0 || (b.group !== null && !q));
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
    {#each blocks as b (b.group?.id ?? 'none')}
      <section>
        {#if b.group}
          {@const g = b.group}
          <label class="ck acc-gh"><input type="checkbox" checked={pickedGroups.includes(g.id)} onchange={() => toggleGroup(g.id)} /><b>{g.name}</b><span class="dim">{t('st.acc.servers.groupNote', { n: servers.filter((s) => s.group_id === g.id).length })}</span></label>
        {:else}<div class="acc-gh"><b>{t('st.acc.servers.ungrouped')}</b></div>{/if}
        {#each b.servers as s (s.id)}
          <label class="ck acc-sr" class:dis={viaGroup(s)}><input type="checkbox" checked={viaGroup(s) || pickedServers.includes(s.id)} disabled={viaGroup(s)} onchange={() => toggleServer(s)} /><span>{s.name}</span><span class="dim mono">{s.address}</span></label>
        {:else}<p class="dim acc-sr">{t('st.acc.servers.empty')}</p>{/each}
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
