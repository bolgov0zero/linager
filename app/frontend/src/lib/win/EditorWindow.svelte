<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi2 } from '../api';
  import { t } from '../i18n.svelte';

  let { id, path }: { id: string; path: string } = $props();

  let text = $state('');
  let orig = '';
  let mtime = 0;
  let status = $state<'loading' | 'ready' | 'saved' | 'error' | 'conflict' | 'toobig'>('loading');
  let busy = $state(false);
  let ta = $state<HTMLTextAreaElement | null>(null);
  let gutter = $state<HTMLDivElement | null>(null);

  const dirty = $derived(status !== 'loading' && text !== orig);
  const lines = $derived(Array.from({ length: text.split('\n').length }, (_, i) => i + 1).join('\n'));
  const name = $derived(path.split('/').pop() ?? path);

  onMount(async () => {
    document.title = `${t('win.editor')} · ${name}`;
    try {
      const c = await serverApi2.fileContent(id, path);
      text = orig = c.content;
      mtime = c.mtime;
      status = 'ready';
    } catch {
      status = 'toobig';
    }
  });

  async function save(): Promise<void> {
    if (busy || !dirty) return;
    busy = true;
    try {
      const r = await serverApi2.fileSave(id, path, text, mtime);
      orig = text;
      mtime = r.mtime;
      status = 'saved';
    } catch (e) {
      status = (e as { status?: number }).status === 409 ? 'conflict' : 'error';
    } finally {
      busy = false;
    }
  }
</script>

<svelte:window onkeydown={(e) => (e.metaKey || e.ctrlKey) && e.key === 's' && (e.preventDefault(), void save())} />

<div class="edw win-full">
  <div class="ed-bar"><span class="mono ed-path">{path}</span>{#if dirty}<span class="ed-mod">{t('win.editor.changed')}</span>{/if}<span class="grow"></span>
    <button class="btn sm primary" type="button" disabled={!dirty || busy} onclick={save}>{t('win.editor.save')}</button></div>
  <div class="ed">
    <div class="ed-g mono" bind:this={gutter}>{lines}</div>
    <textarea bind:this={ta} bind:value={text} spellcheck="false" wrap="off" disabled={status === 'loading' || status === 'toobig'} aria-label={name}
      onscroll={() => gutter && ta && (gutter.scrollTop = ta.scrollTop)}
      onkeydown={(e) => { if (e.key === 'Tab') { e.preventDefault(); const t0 = e.currentTarget; t0.setRangeText('    ', t0.selectionStart, t0.selectionEnd, 'end'); text = t0.value; } }}></textarea>
  </div>
  <div class="ed-s mono">
    <span>UTF-8</span>
    <span class="grow"></span>
    <span class:err-t={status === 'error' || status === 'conflict' || status === 'toobig'}>{status === 'saved' ? t('win.editor.saved') : status === 'conflict' ? t('win.editor.conflict') : status === 'error' ? t('win.editor.error') : status === 'toobig' ? t('win.editor.toobig') : ''}</span>
    <span>{text.split('\n').length} {t('win.editor.lines')}</span>
  </div>
</div>
