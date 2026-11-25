<script lang="ts">
  import type { TagItem } from '../api';
  import { t } from '../i18n.svelte';
  import Icon from '../Icon.svelte';
  import { portal } from '../portal';
  import TagChip from '../TagChip.svelte';
  import { pickerOptions, type PickOption } from './tag-picker';

  // The field "New tag" with its own list in place of the browser's one (a plain black plate that could not be styled). The list
  // shows the tags of the registry that may still be added, in their colours; typing filters it, the arrows choose, Enter adds,
  // Escape closes. A name that is not a tag yet is offered as the last option ("Create the tag").
  let { items, taken = [], value = $bindable(''), placeholder, maxlength, onpick }: {
    items: TagItem[]; taken?: string[]; value?: string; placeholder: string; maxlength: number; onpick: (name: string) => void;
  } = $props();

  let input = $state<HTMLInputElement | null>(null);
  let open = $state(false);
  let active = $state(0);
  let pos = $state({ left: 0, width: 0, top: 0, bottom: 0, up: false, max: 260 });
  const uid = $props.id();

  const options = $derived(pickerOptions(items, value, taken));
  const cur = $derived(Math.min(active, Math.max(0, options.length - 1)));
  const shown = $derived(open && options.length > 0);

  function place(): void {
    if (!input) return;
    const r = input.getBoundingClientRect();
    const below = window.innerHeight - r.bottom - 12;
    const above = r.top - 12;
    const up = below < 170 && above > below;
    pos = { left: r.left, width: r.width, top: r.bottom + 4, bottom: window.innerHeight - r.top + 4, up, max: Math.max(120, Math.min(300, up ? above : below)) };
  }
  function choose(o: PickOption): void {
    value = '';
    active = 0;
    onpick(o.kind === 'tag' ? o.tag.name : o.name);
  }
  function key(e: KeyboardEvent): void {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      if (options.length === 0) return;
      e.preventDefault();
      if (!open) { open = true; place(); return; }
      active = (cur + (e.key === 'ArrowDown' ? 1 : options.length - 1)) % options.length;
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const o = open ? options[cur] : undefined;
      if (o) choose(o);
      else onpick(value);
    } else if (e.key === 'Escape' && open) {
      e.preventDefault();
      e.stopPropagation();
      open = false;
    }
  }
</script>

<svelte:window onresize={() => open && place()} />

<label class="fl"><span class="field">
  <input bind:this={input} autocomplete="off" spellcheck="false" {maxlength} {placeholder} role="combobox" aria-expanded={shown} aria-controls="tp-{uid}" aria-autocomplete="list" aria-activedescendant={shown ? `tp-${uid}-${cur}` : undefined}
    bind:value onfocus={() => { open = true; place(); }} onblur={() => (open = false)} oninput={() => { open = true; active = 0; place(); }} onkeydown={key} />
</span></label>

{#if shown}
  <div class="pop tp-pop" id="tp-{uid}" role="listbox" aria-label={t('ssm.tags')} use:portal={{ near: input, into: 'body' }}
    style="left:{pos.left}px;width:{pos.width}px;max-height:{pos.max}px;{pos.up ? `bottom:${pos.bottom}px` : `top:${pos.top}px`}">
    {#each options as o, i (o.kind === 'tag' ? `t${o.tag.id}` : 'new')}
      <!-- the mouse must not take the focus from the field: the list closes on blur -->
      <button id="tp-{uid}-{i}" type="button" role="option" aria-selected={i === cur} class:act={i === cur} tabindex="-1"
        onmousedown={(e) => e.preventDefault()} onmouseenter={() => (active = i)} onclick={() => choose(o)}>
        {#if o.kind === 'tag'}
          <TagChip name={o.tag.name} color={o.tag.color} />
          {#if i === cur}<kbd>Enter</kbd>{/if}
        {:else}
          <span class="tp-new"><Icon name="plus" size={13} />{t('ssm.tags.create', { name: o.name })}</span>
        {/if}
      </button>
    {/each}
    <div class="tp-ft">{t('ssm.tags.keys')}</div>
  </div>
{/if}
