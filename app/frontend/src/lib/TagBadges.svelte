<script lang="ts">
  import { tagColor } from './tag-colors';
  import { letterOf, splitTags, type BadgeTag } from './tag-badges';

  // The tags of a server as small letter badges. At rest a badge is its first letter on the colour of the tag; the full name slides
  // out to the right on hover, over what stands next to it (the layout does not move). `max` is how many badges the place has
  // room for, "+N" included; the folded tags show their names together when the "+N" is hovered.
  let { tags, max = 3 }: { tags: BadgeTag[]; max?: number } = $props();
  const view = $derived(splitTags(tags, max));
</script>

{#if tags.length > 0}
  <span class="tbs">
    {#each view.shown as tg (tg.name)}
      <span class="tb tc-{tagColor(tg.color)}" role="img" aria-label={tg.name}><span class="tb-i"><span class="tb-l"><i>{letterOf(tg.name)}</i></span><span class="tb-n">{tg.name}</span></span></span>
    {/each}
    {#if view.rest.length > 0}
      <span class="tb tb-more" role="img" aria-label={view.rest.map((x) => x.name).join(', ')}><span class="tb-i"><span class="tb-l"><i>+{view.rest.length}</i></span><span class="tb-n">{view.rest.map((x) => x.name).join(', ')}</span></span></span>
    {/if}
  </span>
{/if}
