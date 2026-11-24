<script lang="ts">
  import type { Group, ServerCard } from './api';
  import { folderOptions } from './folders';
  import CustomSelect from './CustomSelect.svelte';
  import Icon from './Icon.svelte';
  import { t } from './i18n.svelte';

  // The controls of a card in the edit mode of the dashboard: move it before or after the neighbour, give it a group, take
  // it off the dashboard (it can be put back with "Add server") or delete the server.
  let { card, index, count, groups, onmove, ongroup, onremove, ondelete }: {
    card: ServerCard;
    /** The place among the cards that are shown. */
    index: number;
    count: number;
    groups: Group[];
    onmove: (dir: -1 | 1) => void;
    ongroup: (value: string) => void;
    onremove: () => void;
    ondelete: () => void;
  } = $props();

  const options = $derived(folderOptions(groups, t('dash.group.none')));
</script>

<div class="edit-bar">
  <button class="btn icon sm" type="button" disabled={index <= 0} title={t('dash.move.prev')} aria-label={t('dash.move.prev')} onclick={() => onmove(-1)}><Icon name="chevron-left" size={14} /></button>
  <button class="btn icon sm" type="button" disabled={index >= count - 1} title={t('dash.move.next')} aria-label={t('dash.move.next')} onclick={() => onmove(1)}><Icon name="chevron-right" size={14} /></button>
  {#if groups.length > 0}<CustomSelect small value={String(card.group_id ?? 0)} options={options} onchange={ongroup} />{/if}
  <span class="grow"></span>
  <button class="btn icon sm" type="button" title={t('dash.remove')} aria-label={t('dash.remove')} onclick={onremove}><Icon name="x" size={14} /></button>
  <button class="btn icon sm danger" type="button" title={t('dash.deleteServer')} aria-label={t('dash.deleteServer')} onclick={ondelete}><Icon name="trash-2" size={14} /></button>
</div>
