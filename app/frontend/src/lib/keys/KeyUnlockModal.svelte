<script lang="ts">
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import KeyOpenForm from './KeyOpenForm.svelte';

  /** A small window that opens the vault from a form (the RDP form): the file and the passphrase, nothing else. */
  let { onopened, onclose }: { onopened: () => void; onclose: () => void } = $props();
</script>

<Modal title={t('keys.unlock.title')} {onclose}>
  <p class="dim">{t('keys.unlock.hint')}</p>
  <KeyOpenForm {onopened} />
</Modal>
