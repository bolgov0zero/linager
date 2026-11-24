<script lang="ts">
  import Icon from './Icon.svelte';
  import { t } from './i18n.svelte';

  // A search field: the magnifier on the left, a cross on the right that clears it (Escape does the same).
  let { value = $bindable(''), placeholder = '', id, spellcheck = true }: { value: string; placeholder?: string; id?: string; spellcheck?: boolean } = $props();
</script>

<label class="field sf">
  <Icon name="search" size={14} />
  <input {id} {placeholder} bind:value {spellcheck} onkeydown={(e) => e.key === 'Escape' && value && ((value = ''), e.stopPropagation())} />
  {#if value}<button class="clr" type="button" aria-label={t('common.clear')} title={t('common.clear')} onclick={() => (value = '')}><Icon name="x" size={13} /></button>{/if}
</label>
