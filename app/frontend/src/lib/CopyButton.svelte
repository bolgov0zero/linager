<script lang="ts">
  import { t } from './i18n.svelte';
  import Icon from './Icon.svelte';
  import { showToast } from './toast';

  // A small icon button that copies a text to the clipboard and says so (or says it could not).
  let { text, label = '' }: { text: string; label?: string } = $props();

  async function copy(): Promise<void> {
    try {
      await navigator.clipboard.writeText(text);
      showToast(t('st.common.copied'));
    } catch {
      showToast(t('st.common.copyFailed'));
    }
  }
</script>

<button class="btn xs icon" type="button" title={label || t('common.copy')} aria-label={label || t('common.copy')} onclick={copy}><Icon name="copy" size={13} /></button>
