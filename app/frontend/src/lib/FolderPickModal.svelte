<script lang="ts">
  import type { Group } from './api';
  import Icon from './Icon.svelte';
  import { flatten } from './folders';
  import { t } from './i18n.svelte';
  import Modal from './Modal.svelte';

  // Chooses a folder from the tree: a place for a folder or a server. `why` gives the reason a folder cannot be chosen ('' = it can).
  let { title, folders, current, noneLabel, why = () => '', onpick, onclose }: {
    title: string;
    folders: Group[];
    /** The folder it is in now (null = no folder): it is marked and cannot be chosen again. */
    current: number | null;
    noneLabel: string;
    why?: (id: number) => string;
    onpick: (id: number | null) => void;
    onclose: () => void;
  } = $props();

  let chosen = $state<number | null | undefined>(undefined);
  const nodes = $derived(flatten(folders));
</script>

<Modal {title} {onclose}>
  <div class="fpick" role="listbox" aria-label={title}>
    <button type="button" role="option" aria-selected={chosen === null} class:on={chosen === null} disabled={current === null} onclick={() => (chosen = null)}>
      <Icon name="folder-input" size={15} /><span>{noneLabel}</span>{#if current === null}<em>{t('fld.here')}</em>{/if}
    </button>
    {#each nodes as n (n.id)}
      {@const bad = why(n.id)}
      <button type="button" role="option" aria-selected={chosen === n.id} class:on={chosen === n.id} disabled={!!bad || current === n.id} style="padding-left:{8 + (n.depth + 0) * 18}px" onclick={() => (chosen = n.id)}>
        <Icon name="folder" size={15} /><span>{n.name}</span>{#if bad}<em>{bad}</em>{:else if current === n.id}<em>{t('fld.here')}</em>{/if}
      </button>
    {/each}
  </div>
  {#snippet foot()}
    <span class="spacer"></span>
    <button class="btn" type="button" onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" disabled={chosen === undefined} onclick={() => chosen !== undefined && onpick(chosen)}>{t('fld.moveHere')}</button>
  {/snippet}
</Modal>
