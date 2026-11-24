<script lang="ts">
  import { onMount } from 'svelte';
  import { api, serverApi, type TagItem } from '../api';
  import { errorText } from '../errors';
  import { t } from '../i18n.svelte';
  import Icon from '../Icon.svelte';
  import { isAdmin } from '../me.svelte';
  import Modal from '../Modal.svelte';
  import { native, type TrayMode } from '../native';
  import TagChip from '../TagChip.svelte';
  import { showToast } from '../toast';

  // Settings of one server: its tags (kept by the Linager server) and its connections in the tray menu (kept by the
  // application). offered lists what this server can open: the terminal always, remote desktop and Assistant by the agent.
  let { server, offered, nativeApp, onclose, onsaved }: {
    server: { id: string; name: string; tags: TagItem[] };
    offered: TrayMode[];
    nativeApp: boolean;
    onclose: () => void;
    onsaved: (tags: TagItem[]) => void;
  } = $props();

  const MAX_TAGS = 20;
  const MAX_CHARS = 32;
  const canTags = isAdmin();
  const label: Record<TrayMode, string> = { terminal: 'tray.mode.terminal', rdp: 'tray.mode.rdp', asst: 'tray.mode.asst' };
  const icon: Record<TrayMode, string> = { terminal: 'square-terminal', rdp: 'monitor', asst: 'app-window' };

  // svelte-ignore state_referenced_locally
  let tags = $state<string[]>(server.tags.map((x) => x.name)); // a copy to edit: the dialog saves it as a whole
  // svelte-ignore state_referenced_locally
  const saved = server.tags.map((x) => x.name);
  // svelte-ignore state_referenced_locally
  let registry = $state<TagItem[]>([...server.tags]); // the tags that exist (for the colours and the suggestions)
  let draft = $state('');
  let modes = $state<TrayMode[]>([]);
  let loaded = $state(false);
  let busy = $state(false);
  let error = $state('');

  const colorOf = (name: string): string | undefined => registry.find((x) => x.name.toLowerCase() === name.toLowerCase())?.color;
  const suggestions = $derived(registry.filter((x) => !tags.some((n) => n.toLowerCase() === x.name.toLowerCase())).map((x) => x.name));

  onMount(async () => {
    if (canTags) {
      try {
        registry = (await api.tags()).tags;
      } catch {
        // the suggestions are a convenience: typing a name still works
      }
    }
    if (nativeApp) {
      try {
        const r = await native.trayGet();
        modes = r.entries.find((e) => e.server_id === server.id)?.modes.filter((m) => offered.includes(m)) ?? [];
      } catch {
        // the tray is not available: the list stays empty
      }
    }
    loaded = true;
  });

  function addTag(): void {
    const v = draft.split(/\s+/).filter(Boolean).join(' ');
    if (!v) return;
    if (v.length > MAX_CHARS) {
      error = t('ssm.tags.long', { n: MAX_CHARS });
      return;
    }
    if (tags.length >= MAX_TAGS) {
      error = t('ssm.tags.many', { n: MAX_TAGS });
      return;
    }
    error = '';
    if (!tags.some((x) => x.toLowerCase() === v.toLowerCase())) tags = [...tags, v];
    draft = '';
  }

  function toggle(m: TrayMode): void {
    modes = modes.includes(m) ? modes.filter((x) => x !== m) : [...modes, m];
  }

  async function save(): Promise<void> {
    if (draft.trim()) addTag();
    if (error) return;
    busy = true;
    error = '';
    try {
      if (canTags && JSON.stringify(tags) !== JSON.stringify(saved)) {
        const r = await serverApi.setTags(server.id, tags);
        onsaved(r.tag_items);
      }
      if (nativeApp) await native.traySet(server.id, server.name, modes);
      showToast(t('ssm.saved'));
      onclose();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t('ssm.title', { name: server.name })} {onclose}>
  <form class="mdl-stack" onsubmit={(e) => { e.preventDefault(); void save(); }}>
    <section class="mdl-stack">
      <b>{t('ssm.tags')}</b>
      <p class="dim">{t('ssm.tags.hint')}</p>
      {#if tags.length}
        <div class="chips">
          {#each tags as tg (tg)}
            <TagChip name={tg} color={colorOf(tg)}>{#if canTags}<button class="btn xs" type="button" aria-label={t('ssm.tags.remove', { tag: tg })} onclick={() => (tags = tags.filter((x) => x !== tg))}><Icon name="x" size={12} /></button>{/if}</TagChip>
          {/each}
        </div>
      {:else}<p class="dim">{t('ssm.tags.none')}</p>{/if}
      {#if canTags}
        <label class="fl"><span class="field"><input autocomplete="off" spellcheck="false" maxlength={MAX_CHARS} placeholder={t('ssm.tags.add')} list="ssm-tags-{server.id}" bind:value={draft} onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addTag(); } }} /></span></label>
        <datalist id="ssm-tags-{server.id}">{#each suggestions as sg (sg)}<option value={sg}></option>{/each}</datalist>
      {:else}<p class="dim">{t('ssm.tags.admin')}</p>{/if}
    </section>
    <section class="mdl-stack">
      <b>{t('ssm.tray')}</b>
      <p class="dim">{t('ssm.tray.hint')}</p>
      {#if !nativeApp}
        <p class="dim">{t('ssm.tray.browser')}</p>
      {:else}
        {#each offered as m (m)}
          <label class="ck"><input type="checkbox" disabled={!loaded} checked={modes.includes(m)} onchange={() => toggle(m)} /><Icon name={icon[m]} size={14} /><span>{t(label[m])}</span></label>
        {/each}
      {/if}
    </section>
    {#if error}<div class="err" role="alert">{error}</div>{/if}
    <div class="row end">
      <button class="btn" type="button" disabled={busy} onclick={onclose}>{t('common.cancel')}</button>
      <button class="btn primary" type="submit" disabled={busy}>{t('common.save')}</button>
    </div>
  </form>
</Modal>
