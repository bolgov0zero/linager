<script lang="ts">
  import { ICONS } from './icons';

  let { name, size = 16 }: { name: string; size?: number } = $props();
</script>

<svg class="ic" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">{@html ICONS[name] ?? ''}</svg>
