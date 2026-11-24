<script lang="ts">
  import type { Snippet } from 'svelte';
  import { tagColor } from './tag-colors';

  // A tag as a coloured label. An unknown colour is drawn gray. The optional children go after the name (a remove button).
  let { name, color, children }: { name: string; color?: string; children?: Snippet } = $props();
</script>

<span class="tlab tc-{tagColor(color)}"><span>{name}</span>{@render children?.()}</span>
