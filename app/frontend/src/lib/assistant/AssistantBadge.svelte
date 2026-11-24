<script lang="ts">
  import { t } from '../i18n.svelte';
  import { isSecureDesktop } from './protocol';

  /** The warning that the computer shows the lock, sign-in or UAC screen (what is seen is not the desktop of the user). */
  let { desktop }: { desktop?: string | null } = $props();
</script>

{#if isSecureDesktop(desktop)}<span class="badge b-warn" role="status">{t('assist.secureDesktop')}</span>{/if}
