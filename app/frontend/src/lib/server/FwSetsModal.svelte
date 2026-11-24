<script lang="ts">
  import type { FwSet } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import SearchField from '../SearchField.svelte';
  import { parseAddresses, usedIn } from '../fw-sets';

  // The address lists (nftables sets) of the draft: rules refer to them as @name.
  let { sets, rules, onchange, onclose }: { sets: FwSet[]; rules: string[]; onchange: (next: FwSet[]) => void; onclose: () => void } = $props();

  let selected = $state('');
  let newName = $state('');
  let newFamily = $state<'ipv4' | 'ipv6'>('ipv4');
  let addText = $state('');
  let nameError = $state('');
  let addError = $state('');
  let search = $state('');
  const NAME_RE = /^[a-z][a-z0-9_]{0,30}$/;

  const current = $derived(sets.find((x) => x.name === selected) ?? sets[0] ?? null);
  const shownEls = $derived(current ? (search.trim() ? current.elements.filter((e) => e.includes(search.trim())) : current.elements) : []);
  const uses = $derived(current ? usedIn(rules, current.name) : 0);
  const replace = (name: string, fn: (s: FwSet) => FwSet): void => onchange(sets.map((x) => (x.name === name ? fn(x) : x)));

  function createList(): void {
    const name = newName.trim();
    if (!NAME_RE.test(name)) return void (nameError = t('fw.sets.errName'));
    if (sets.some((x) => x.name === name)) return void (nameError = t('fw.sets.errExists'));
    nameError = '';
    onchange([...sets, { name, family: newFamily, elements: [] }]);
    selected = name;
    newName = '';
  }
  function removeList(): void {
    if (!current || uses > 0) return;
    const name = current.name;
    onchange(sets.filter((x) => x.name !== name));
    selected = '';
  }
  function addAddresses(): void {
    if (!current) return;
    const res = parseAddresses(addText, current.family);
    if (res.bad.length) return void (addError = res.bad.slice(0, 3).join(', '));
    addError = '';
    const known = new Set(current.elements);
    const fresh = res.items.filter((x) => !known.has(x));
    if (fresh.length) replace(current.name, (x) => ({ ...x, elements: [...x.elements, ...fresh] }));
    addText = '';
  }
  const removeEl = (el: string): void => {
    if (current) replace(current.name, (x) => ({ ...x, elements: x.elements.filter((e) => e !== el) }));
  };
</script>

<Modal title={t('fw.sets.title')} wide {onclose}>
  <p class="dim">{t('fw.sets.help')}</p>
  <div class="sets-grid">
    <div class="side">
      {#each sets as x (x.name)}
        <button class="item" type="button" aria-pressed={current?.name === x.name} onclick={() => { selected = x.name; search = ''; addError = ''; }}><span class="mono">@{x.name}</span><span class="dim">{x.family === 'ipv6' ? 'IPv6' : 'IPv4'} · {x.elements.length}</span></button>
      {:else}<p class="dim">{t('fw.sets.none')}</p>{/each}
      <form class="two" style="grid-template-columns:1fr" onsubmit={(e) => { e.preventDefault(); createList(); }}>
        <span class="field"><input class="mono" placeholder={t('fw.sets.newName')} aria-label={t('fw.sets.newName')} spellcheck="false" autocomplete="off" bind:value={newName} oninput={() => (nameError = '')} /></span>
        <div class="row"><CustomSelect small value={newFamily} options={[{ v: 'ipv4', t: 'IPv4' }, { v: 'ipv6', t: 'IPv6' }]} onchange={(v) => (newFamily = v as 'ipv4' | 'ipv6')} />
          <button class="btn sm primary" type="submit">{t('fw.sets.create')}</button></div>
      </form>
      {#if nameError}<div class="err">{nameError}</div>{/if}
    </div>
    <div class="main" style="display:grid;gap:10px;align-content:start">
      {#if current}
        <div class="row"><b class="mono">@{current.name}</b><span class="dim">{current.family === 'ipv6' ? 'IPv6' : 'IPv4'} · {uses > 0 ? t('fw.sets.usedIn', { n: uses }) : t('fw.sets.unused')}</span><span class="grow"></span>
          <button class="btn sm danger" type="button" disabled={uses > 0} title={uses > 0 ? t('fw.sets.deleteBlocked') : undefined} onclick={removeList}>{t('fw.sets.delete')}</button></div>
        <form class="row" onsubmit={(e) => { e.preventDefault(); addAddresses(); }}>
          <span class="field grow"><input class="mono" placeholder={current.family === 'ipv6' ? '2001:db8::/32' : '198.51.100.20, 10.20.0.0/24'} aria-label={t('fw.sets.addLabel')} spellcheck="false" autocomplete="off" bind:value={addText} oninput={() => (addError = '')} /></span>
          <button class="btn primary" type="submit" disabled={!addText.trim()}>{t('fw.sets.add')}</button>
        </form>
        {#if addError}<div class="err">{t('fw.sets.errAddr', { items: addError })}</div>{/if}
        {#if current.elements.length > 8}<SearchField placeholder={t('fw.sets.search')} bind:value={search} spellcheck={false} />{/if}
        <div class="elems">
          {#each shownEls as e (e)}<span class="gtag">{e}<button type="button" class="x" aria-label={t('fw.sets.remove', { item: e })} onclick={() => removeEl(e)}><Icon name="x" size={11} /></button></span>
          {:else}<span class="dim">{current.elements.length ? t('fw.sets.nothingFound') : t('fw.sets.emptyList')}</span>{/each}
        </div>
        <p class="dim">{t('fw.sets.draftNote')}</p>
      {:else}<p class="dim">{t('fw.sets.pickOrCreate')}</p>{/if}
    </div>
  </div>
  {#snippet foot()}<span class="spacer"></span><button class="btn" type="button" onclick={onclose}>{t('common.close')}</button>{/snippet}
</Modal>
