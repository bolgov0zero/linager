<script lang="ts">
  import { onMount } from 'svelte';
  import CustomSelect from '../CustomSelect.svelte';
  import { confirmAsk } from '../dialogs.svelte';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import SearchField from '../SearchField.svelte';
  import { showToast } from '../toast';
  import KeyEditModal from './KeyEditModal.svelte';
  import KeyOpenForm from './KeyOpenForm.svelte';
  import { TYPES, bindText } from './labels';
  import { LOCK_CHOICES, autosave, closeVault, createVault, flush, lockMinutes, lockWaiting, removeEntry, saveFile, setLockMinutes, touch, vault } from './store';
  import { DEFAULT_FILE, PASSPHRASE_MIN, VaultError, accountOf, type Entry, type EntryType } from './vault';

  // The window "Keys": create or open the personal vault, then list, add, change and delete the keys.
  let { onclose }: { onclose: () => void } = $props();

  // ---- the vault is closed ----
  let p1 = $state('');
  let p2 = $state('');
  let createBusy = $state(false);
  let createError = $state('');
  const strength = $derived(p1.length < 8 ? 1 : p1.length < 12 ? 2 : p1.length < 16 ? 3 : 4);
  const createHint = $derived(p1.length > 0 && p1.length < PASSPHRASE_MIN ? t('keys.create.short') : p2 && p1 !== p2 ? t('keys.create.mismatch') : t('keys.create.hint', { n: PASSPHRASE_MIN }));
  const canCreate = $derived(p1.length >= PASSPHRASE_MIN && p1 === p2 && !createBusy);

  async function create(): Promise<void> {
    if (!canCreate) return;
    createBusy = true;
    createError = '';
    try {
      if (await createVault(p1)) {
        showToast(t('keys.created', { name: $vault?.file ?? DEFAULT_FILE }));
        p1 = p2 = '';
      }
    } catch (e) {
      createError = e instanceof VaultError ? t(`keys.err.${e.code}`) : t('common.genericError');
    } finally {
      createBusy = false;
    }
  }

  // ---- the vault is open ----
  let filter = $state<'all' | EntryType>('all');
  let query = $state('');
  let editing = $state<Entry | 'new' | null>(null);
  const entries = $derived($vault?.entries ?? []);
  const shown = $derived(entries.filter((e) => filter === 'all' || e.type === filter).filter((e) => {
    const q = query.trim().toLowerCase();
    return !q || `${e.name} ${e.login} ${e.domain} ${bindText(e, t)}`.toLowerCase().includes(q);
  }).sort((a, b) => a.name.localeCompare(b.name)));
  const lockOptions = $derived(LOCK_CHOICES.map((m) => ({ v: String(m), t: m === 0 ? t('keys.lock.off') : t('keys.lock.m', { n: m }) })));

  async function save(): Promise<boolean> {
    const ok = await saveFile();
    if (ok) showToast(t('keys.savedToast', { name: $vault?.file ?? '' }));
    return ok;
  }
  async function closeAsk(): Promise<void> {
    if ($vault?.dirty) await flush(); // the last changes go into the file before it closes
    if (!$vault?.dirty) return closeVault();
    // saving first; "drop" closes at once
    const keep = await confirmAsk({ title: t('keys.closeAsk.title'), text: t('keys.closeAsk.text'), ok: t('keys.closeAsk.save'), cancel: t('keys.closeAsk.drop'), danger: false });
    if (keep) {
      if (await save()) closeVault();
    } else closeVault();
  }
  async function remove(e: Entry): Promise<void> {
    if (await confirmAsk({ title: t('keys.delete'), text: `${t('keys.delete.ask')} «${e.name}»`, ok: t('keys.delete'), danger: true })) removeEntry(e.id);
  }
  onMount(() => touch());
</script>

<Modal title={t('keys.title')} wide {onclose}>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="mdl-stack" onpointerdown={touch} onkeydown={touch}>
    {#if !$vault}
      <p class="dim">{t('keys.intro')}</p>
      <div class="cols2">
        <form class="mdl-stack" onsubmit={(e) => { e.preventDefault(); void create(); }}>
          <div class="sh">{t('keys.create.title')}</div>
          <label class="fl"><span>{t('keys.pass')}</span><span class="field"><input type="password" bind:value={p1} autocomplete="new-password" /></span>
            <span class="meter" aria-hidden="true">{#each [1, 2, 3, 4] as n (n)}<i class:on={p1.length > 0 && n <= strength} class:w={strength === 3} class:c={strength < 3}></i>{/each}</span></label>
          <label class="fl"><span>{t('keys.pass2')}</span><span class="field"><input type="password" bind:value={p2} autocomplete="new-password" /></span></label>
          <p class="warnbox warn">{t('keys.create.warn')}</p>
          <p class="dim">{createHint}</p>
          {#if createError}<div class="err" role="alert">{createError}</div>{/if}
          <button class="btn primary" type="submit" disabled={!canCreate}>{createBusy ? t('keys.create.busy') : t('keys.create.go')}</button>
        </form>
        <div class="mdl-stack"><div class="sh">{t('keys.open.title')}</div><KeyOpenForm /></div>
      </div>
    {:else}
      <div class="keys-strip">
        <span class="ok-t"><span class="dot ok"></span> {t('keys.state.open')}</span>
        <b class="mono" title={$vault.file}>{$vault.file}</b>
        <span class="dim">{t('keys.count', { n: entries.length })}</span>
        {#if $vault.saveError}<span class="badge b-crit">{t('keys.saveFailed')}</span>
        {:else if $vault.dirty || $vault.saving}<span class="dim">{t('keys.saving')}</span>
        {:else}<span class="dim">{t('keys.saved')}</span>{/if}
        <span class="grow"></span>
        <span class="dim">{t('keys.lock.label')}</span><CustomSelect small value={String($lockMinutes)} options={lockOptions} onchange={(v) => setLockMinutes(Number(v))} />
        {#if $vault.saveError || !$autosave}<button class="btn sm primary" type="button" onclick={() => void save()}>{t('keys.save')}</button>{/if}
        <button class="btn sm" type="button" onclick={closeAsk}>{t('keys.close')}</button>
      </div>
      {#if $lockWaiting}<p class="warnbox warn" role="status">{t('keys.lock.waiting')}</p>{/if}
      <div class="row">
        <SearchField placeholder={t('keys.search')} bind:value={query} spellcheck={false} />
        <button class="chip" type="button" aria-pressed={filter === 'all'} onclick={() => (filter = 'all')}>{t('keys.filter.all')}</button>
        {#each TYPES as ty (ty)}<button class="chip" type="button" aria-pressed={filter === ty} onclick={() => (filter = ty)}>{t(`keys.type.${ty}`)}</button>{/each}
        <span class="grow"></span>
        <button class="btn primary" type="button" onclick={() => (editing = 'new')}>{t('keys.add')}</button>
      </div>
      {#if entries.length === 0}<p class="dim">{t('keys.empty')}</p>
      {:else}
        <div class="tw"><table class="tbl compact">
          <thead><tr><th>{t('keys.col.name')}</th><th>{t('keys.col.use')}</th><th>{t('keys.col.bind')}</th><th>{t('keys.col.login')}</th><th></th></tr></thead>
          <tbody>
            {#each shown as e (e.id)}
              <tr><td><b>{e.name}</b></td><td><span class="badge ty-{e.type}">{t(`keys.type.${e.type}`)}</span></td><td>{bindText(e, t)}</td><td class="mono">{accountOf(e)}</td>
                <td class="r"><span class="r-btns"><button class="btn sm" type="button" onclick={() => (editing = e)}>{t('keys.edit')}</button><button class="btn sm danger" type="button" onclick={() => remove(e)}>{t('keys.delete')}</button></span></td></tr>
            {:else}<tr><td colspan="5" class="dim">{t('keys.nomatch')}</td></tr>{/each}
          </tbody>
        </table></div>
      {/if}
      <p class="dim">{t('keys.legend')}</p>
    {/if}
  </div>
</Modal>
{#if editing}<KeyEditModal entry={editing === 'new' ? null : editing} onclose={() => (editing = null)} />{/if}
