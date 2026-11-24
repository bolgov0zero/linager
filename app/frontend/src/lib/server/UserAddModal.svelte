<script lang="ts">
  import { serverApi2, type HostUsersState } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { errorText } from '../errors';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import { genPassword, HOT_GROUPS } from '../users-util';

  let { serverId, info, onclose, ondone }: { serverId: string; info: HostUsersState; onclose: () => void; ondone: (msg: string) => void } = $props();

  let name = $state('');
  let gecos = $state('');
  let shell = $state('/bin/bash');
  let home = $state('');
  let pwMode = $state<'generate' | 'manual' | 'none'>('generate');
  let generated = $state(genPassword());
  let manual = $state('');
  let groups = $state<string[]>([]);
  let sudo = $state<'none' | 'pw' | 'nopw'>('none');
  let sshKey = $state('');
  let busy = $state(false);
  let error = $state('');
  let copied = $state(false);

  const nameOk = $derived(/^[a-z_][a-z0-9_-]{0,31}$/.test(name));
  const taken = $derived(info.users.some((u) => u.name === name));
  const password = $derived(pwMode === 'generate' ? generated : pwMode === 'manual' ? manual : '');
  const pwOk = $derived(pwMode === 'none' || (password.length >= 8 && password.length <= 128));
  const shellOptions = $derived(info.shells.map((s) => ({ v: s, t: s })));
  const homePath = $derived(home || `/home/${name || 'login'}`);

  /** The commands the agent will run, shown before the account is created (the password is not part of them). */
  const preview = $derived.by(() => {
    const all = sudo === 'none' ? groups : [...groups, info.sudo_group];
    const lines = [`useradd --create-home --home-dir ${homePath} --shell ${shell} --user-group${gecos ? ` --comment "${gecos}"` : ''}${all.length ? ` --groups ${all.join(',')}` : ''} ${name || 'login'}`];
    if (password) lines.push(`chpasswd   # ${t('usr.add.previewPw')}`);
    if (sudo === 'nopw') lines.push(`/etc/sudoers.d/linager-${name || 'login'}   # NOPASSWD: ALL`);
    if (sshKey.trim()) lines.push(`${homePath}/.ssh/authorized_keys`);
    return lines.join('\n');
  });

  const toggle = (g: string): void => {
    groups = groups.includes(g) ? groups.filter((x) => x !== g) : [...groups, g];
  };

  async function copy(): Promise<void> {
    try {
      await navigator.clipboard.writeText(generated);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {
      // a blocked clipboard: the password stays on screen
    }
  }

  async function create(): Promise<void> {
    if (busy || !nameOk || taken || !pwOk) return;
    busy = true;
    error = '';
    try {
      const res = await serverApi2.hostUserAdd(serverId, { name, gecos, shell, home, groups, sudo, password, ssh_key: sshKey.trim() });
      ondone(res.warnings.length > 0 ? t('usr.add.doneWarn', { user: name, w: res.warnings.join('; ') }) : t('usr.add.done', { user: name }));
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t('usr.add.title')} wide {onclose}>
  <div class="two">
    <label class="fl"><span>{t('usr.add.login')}</span><span class="field"><input bind:value={name} autocomplete="off" spellcheck="false" autocapitalize="none" /></span>
      {#if name && !nameOk}<span class="err-t">{t('usr.add.loginRule')}</span>{:else if taken}<span class="err-t">{t('usr.add.taken')}</span>{/if}</label>
    <label class="fl"><span>{t('usr.add.gecos')}</span><span class="field"><input bind:value={gecos} maxlength="100" /></span></label>
  </div>
  <div class="two">
    <div class="fl"><span>{t('usr.props.shell')}</span><CustomSelect value={shell} options={shellOptions} onchange={(v) => (shell = v)} /></div>
    <label class="fl"><span>{t('usr.props.home')}</span><span class="field"><input class="mono" bind:value={home} placeholder={`/home/${name || 'login'}`} spellcheck="false" /></span></label>
  </div>
  <div class="fl"><span>{t('usr.props.sudo')}</span>
    <div class="seg sm">
      <button type="button" aria-pressed={sudo === 'none'} onclick={() => (sudo = 'none')}>{t('usr.sudo.none')}</button>
      <button type="button" aria-pressed={sudo === 'pw'} onclick={() => (sudo = 'pw')}>{t('usr.sudo.pw')}</button>
      <button type="button" aria-pressed={sudo === 'nopw'} onclick={() => (sudo = 'nopw')}>{t('usr.sudo.nopw')}</button>
    </div>
    {#if sudo === 'nopw'}<span class="err-t">{t('usr.sudo.nopwHint')}</span>{/if}
  </div>
  <div class="fl"><span>{t('usr.props.groups')}</span>
    <div class="chk-grid">
      {#each info.groups as g (g)}<label class="ck"><input type="checkbox" checked={groups.includes(g)} onchange={() => toggle(g)} />{g}{#if HOT_GROUPS.includes(g)}<em class="hot">{t('usr.props.hot')}</em>{/if}</label>{/each}
    </div>
  </div>
  <div class="fl"><span>{t('usr.props.password')}</span>
    <div class="opts">
      <button class="optc" type="button" aria-pressed={pwMode === 'generate'} onclick={() => (pwMode = 'generate')}>{t('usr.pw.generate')}<small>{t('usr.pw.generateHint')}</small></button>
      <button class="optc" type="button" aria-pressed={pwMode === 'manual'} onclick={() => (pwMode = 'manual')}>{t('usr.pw.manual')}</button>
      <button class="optc" type="button" aria-pressed={pwMode === 'none'} onclick={() => (pwMode = 'none')}>{t('usr.add.pwNone')}<small>{t('usr.add.pwNoneHint')}</small></button>
    </div>
    {#if pwMode === 'generate'}
      <div class="row">
        <span class="field grow"><input class="mono" readonly value={generated} aria-label={t('usr.pw.generated')} /></span>
        <button class="btn sm" type="button" onclick={copy}>{copied ? t('usr.pw.copied') : t('common.copy')}</button>
        <button class="btn sm" type="button" onclick={() => (generated = genPassword())}>{t('usr.pw.again')}</button>
      </div>
      <span class="dim">{t('usr.pw.once')}</span>
    {:else if pwMode === 'manual'}
      <span class="field"><input type="password" autocomplete="new-password" bind:value={manual} aria-label={t('usr.pw.new')} /></span>
      <span class="dim">{t('usr.pw.rule')}</span>
    {/if}
  </div>
  <label class="fl"><span>{t('usr.add.key')}</span><textarea class="ta" rows="2" bind:value={sshKey} placeholder="ssh-ed25519 AAAA… user@host" spellcheck="false"></textarea></label>
  <div class="fl"><span>{t('usr.add.preview')}</span><pre class="pv">{preview}</pre></div>
  {#if error}<div class="err" role="alert">{error}</div>{/if}
  {#snippet foot()}
    <span class="spacer"></span>
    <button class="btn" type="button" onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" disabled={busy || !nameOk || taken || !pwOk} onclick={create}>{t('usr.add.create')}</button>
  {/snippet}
</Modal>
