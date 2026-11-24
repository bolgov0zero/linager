<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { serverApi } from '../api';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import { renderMarkdown } from '../markdown';

  let { id, title }: { id: string; title: string } = $props();

  let text = $state('');
  let loaded = $state(false);
  // a note with text opens rendered (Markdown); an empty one opens for typing
  let editing = $state(false);
  let ta = $state<HTMLTextAreaElement | null>(null);
  const html = $derived(renderMarkdown(text));
  let status = $state<'saved' | 'saving' | 'error'>('saved');
  let timer: ReturnType<typeof setTimeout> | undefined;
  const LIMIT = 20000;

  onMount(async () => {
    document.title = `${t('win.note')} · ${title}`;
    try {
      text = (await serverApi.detail(id)).note ?? '';
    } catch {
      status = 'error';
    }
    loaded = true;
    editing = text.trim() === '';
  });

  async function toggle(): Promise<void> {
    editing = !editing;
    if (editing) {
      await tick();
      ta?.focus();
    }
  }

  // the note is saved by itself shortly after the typing stops
  function changed(): void {
    status = 'saving';
    clearTimeout(timer);
    timer = setTimeout(async () => {
      try {
        await serverApi.noteSet(id, text);
        status = 'saved';
      } catch {
        status = 'error';
      }
    }, 700);
  }
</script>

<div class="note-win">
  {#if editing}
    <textarea bind:this={ta} bind:value={text} oninput={changed} placeholder={t('win.note.ph')} maxlength={LIMIT} disabled={!loaded} spellcheck="false" aria-label={t('win.note')}></textarea>
  {:else}
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div class="md note-md" role="document" ondblclick={toggle}>{@html html}</div>
  {/if}
  <div class="note-bar">
    <span class:err-t={status === 'error'}>{status === 'saved' ? t('win.note.saved') : status === 'saving' ? t('win.note.saving') : t('win.note.error')}</span>
    <span class="spacer"></span>
    {#if editing}<span>{text.length} / {LIMIT}</span>{/if}
    <button class="btn xs" type="button" disabled={!loaded} title={editing ? t('win.note.view') : t('win.note.edit')} aria-label={editing ? t('win.note.view') : t('win.note.edit')} onclick={toggle}><Icon name={editing ? 'eye' : 'pencil'} size={14} /></button>
  </div>
</div>
