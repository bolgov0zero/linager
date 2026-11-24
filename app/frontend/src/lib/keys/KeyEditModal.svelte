<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import KeyBindModal from './KeyBindModal.svelte';
  import { TYPES, bindText } from './labels';
  import { loadDirectory, putEntry, type Directory } from './store';
  import { LIMITS, blankEntry, type Entry, type EntryType, type Ref } from './vault';

  // Adds a key (`entry` is null) or changes one. The change goes into the open vault in memory; the file is saved from the window.
  let { entry, onclose }: { entry: Entry | null; onclose: () => void } = $props();

  const start = untrack(() => entry ?? blankEntry());
  let name = $state(start.name);
  let type = $state<EntryType>(start.type);
  let groups = $state<Ref[]>(start.groups.map((g) => ({ ...g })));
  let servers = $state<Ref[]>(start.servers.map((s) => ({ ...s })));
  let login = $state(start.login);
  let domain = $state(start.domain);
  let password = $state(start.password);
  let note = $state(start.note);
  let shown = $state(false);
  let error = $state('');
  let dir = $state<Directory | null>(null);
  let dirFailed = $state(false);
  let picking = $state(false);

  onMount(() => {
    loadDirectory().then((d) => (dir = d), () => (dirFailed = true));
  });

  // what the panel does not know any more stays in the key until it is taken off here
  const goneGroups = $derived(dir ? groups.filter((g) => !dir!.groups.some((x) => String(x.id) === g.id)) : []);
  const goneServers = $derived(dir ? servers.filter((s) => !dir!.servers.some((x) => x.id === s.id)) : []);

  function applyBinding(pickedServers: string[], pickedGroups: number[]): void {
    if (!dir) return;
    const d = dir;
    const groupSet = new Set(pickedGroups);
    const keepG = groups.filter((g) => !d.groups.some((x) => String(x.id) === g.id));
    const keepS = servers.filter((s) => !d.servers.some((x) => x.id === s.id));
    groups = [...pickedGroups.map((id) => ({ id: String(id), name: d.groups.find((g) => g.id === id)?.name ?? String(id) })), ...keepG];
    // a server of a chosen group is covered by the group
    servers = [
      ...pickedServers.filter((id) => {
        const gid = d.servers.find((s) => s.id === id)?.group_id;
        return gid == null || !groupSet.has(gid);
      }).map((id) => ({ id, name: d.servers.find((s) => s.id === id)?.name ?? id })),
      ...keepS,
    ];
    picking = false;
  }
  const forget = (kind: 'g' | 's', id: string): void => {
    if (kind === 'g') groups = groups.filter((g) => g.id !== id);
    else servers = servers.filter((s) => s.id !== id);
  };

  const ALPHABET = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';
  function generate(): void {
    const bytes = new Uint8Array(20);
    crypto.getRandomValues(bytes);
    password = Array.from(bytes, (b) => ALPHABET[b % ALPHABET.length]).join('');
    shown = true;
  }
  function submit(): void {
    if (!name.trim() || !login.trim()) return void (error = t('keys.edit.need'));
    if (!putEntry({ id: start.id, name: name.trim(), type, groups, servers, login: login.trim(), domain: domain.trim(), password, note: note.trim() })) return void (error = t('keys.err.limit'));
    onclose();
  }
</script>

<Modal title={entry ? t('keys.edit.title', { name: entry.name }) : t('keys.edit.new')} {onclose}>
  <label class="fl"><span>{t('keys.f.name')}</span><span class="field"><input bind:value={name} maxlength={LIMITS.name} placeholder={t('keys.f.name.ph')} autocomplete="off" /></span></label>
  <div class="fl"><span>{t('keys.f.use')}</span>
    <div class="row">{#each TYPES as ty (ty)}<button class="chip" type="button" aria-pressed={type === ty} onclick={() => (type = ty)}>{t(`keys.type.${ty}`)}</button>{/each}</div>
    <small>{t(`keys.use.${type}`)}</small></div>
  <div class="fl"><span>{t('keys.f.bind')}</span>
    <button class="btn" type="button" style="justify-content:space-between" disabled={!dir} onclick={() => (picking = true)}><span>{bindText({ groups, servers }, t)}</span><span class="dim">{t('keys.f.bind.change')}</span></button>
    {#if dirFailed}<span class="err-t">{t('keys.dirError')}</span>{/if}
    {#if goneGroups.length + goneServers.length}
      <div class="row"><small>{t('keys.gone.label')}</small>
        {#each goneGroups as g (g.id)}<button class="btn xs" style="width:auto;padding:0 8px" type="button" onclick={() => forget('g', g.id)}>{t('keys.gone.remove', { name: g.name })}</button>{/each}
        {#each goneServers as s (s.id)}<button class="btn xs" style="width:auto;padding:0 8px" type="button" onclick={() => forget('s', s.id)}>{t('keys.gone.remove', { name: s.name })}</button>{/each}</div>
    {/if}</div>
  <div class="two">
    <label class="fl"><span>{t('keys.f.login')}</span><span class="field"><input bind:value={login} maxlength={LIMITS.login} autocomplete="off" spellcheck="false" autocapitalize="off" /></span></label>
    <label class="fl"><span>{t('keys.f.domain')}</span><span class="field"><input bind:value={domain} maxlength={LIMITS.domain} autocomplete="off" spellcheck="false" autocapitalize="off" /></span></label>
  </div>
  <div class="fl"><span>{t('keys.f.pass')}</span>
    <div class="row"><span class="field grow"><input class="mono" type={shown ? 'text' : 'password'} bind:value={password} maxlength={LIMITS.password} autocomplete="new-password" spellcheck="false" /></span>
      <button class="btn" type="button" onclick={() => (shown = !shown)}>{shown ? t('keys.pass.hide') : t('keys.pass.show')}</button>
      <button class="btn" type="button" onclick={generate}>{t('keys.pass.gen')}</button></div></div>
  <label class="fl"><span>{t('keys.f.note')}</span><textarea class="ta" rows="2" bind:value={note} maxlength={LIMITS.note}></textarea></label>
  <p class="dim">{t('keys.edit.memo')}</p>
  {#if error}<div class="err" role="alert">{error}</div>{/if}
  {#snippet foot()}
    <span class="spacer"></span>
    <button class="btn" type="button" onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" onclick={submit}>{t('keys.edit.done')}</button>
  {/snippet}
</Modal>
{#if picking && dir}<KeyBindModal {dir} servers={servers.map((s) => s.id)} groups={groups.map((g) => Number(g.id)).filter((n) => Number.isFinite(n))} onapply={applyBinding} onclose={() => (picking = false)} />{/if}
