<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { serverApi2, type DockerContainer, type DockerEditConfig } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { errorText } from '../docker-util';
  import { cloneDraft, diffSpec, isSecretName, newEnv, newMount, newPort, tabOfPath, toDraft, toSpec, usedPorts, validateSpec, type Draft, type DiffWords, type EditTab } from '../docker-edit';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import { fmtBytes } from '../status';

  let { serverId, container, config, others, onclose, onapply }: {
    serverId: string; container: DockerContainer; config: DockerEditConfig; others: DockerContainer[]; onclose: () => void; onapply: (token: string) => void;
  } = $props();

  const TABS: EditTab[] = ['main', 'ports', 'mounts', 'env', 'res'];
  const RESTART = ['no', 'always', 'unless-stopped', 'on-failure'] as const;
  // the dialog edits a copy of the settings it was opened with
  const orig: Draft = untrack(() => toDraft(config));
  const compose = untrack(() => config.compose);
  const running = untrack(() => config.running);
  let draft = $state<Draft>(cloneDraft(orig));
  let tab = $state<EditTab>('main');
  let phase = $state<'edit' | 'review'>('edit');
  let confirmDiscard = $state(false);
  let applying = $state(false);
  let applyError = $state('');
  let reveal = $state<Record<number, boolean>>({});

  const words = $derived<DiffWords>({ asImage: t('dockerEdit.words.asImage'), unlimited: t('dockerEdit.words.unlimited'), mb: t('dockerEdit.words.mb'), cores: t('dockerEdit.words.cores'), readOnly: t('dockerEdit.words.readOnly'), restart: { no: 'no', always: 'always', 'unless-stopped': 'unless-stopped', 'on-failure': 'on-failure' } });
  const diff = $derived(diffSpec(orig, draft, words));
  const taken = $derived(usedPorts(others));
  const errors = $derived(validateSpec(draft, { orig, compose: !!compose, others: taken, t: (k, v) => t(k, v) }));
  const firstError = $derived(Object.values(errors)[0] ?? '');
  const count = $derived(diff.changes.length);
  const errorTabs = $derived(new Set(Object.keys(errors).map(tabOfPath)));
  const restartOptions = $derived(RESTART.map((v) => ({ v, t: t(`dockerEdit.restart.${v}`) })));
  const protoOptions = [{ v: 'tcp', t: 'tcp' }, { v: 'udp', t: 'udp' }];
  const mountTypes = $derived([{ v: 'bind', t: t('dockerEdit.mount.bind') }, { v: 'volume', t: t('dockerEdit.mount.volume') }]);
  const origIds = new Set<number>([...orig.ports, ...orig.mounts, ...orig.env].map((r) => r._id));
  const signature = $derived(JSON.stringify(draft));
  $effect(() => {
    void signature;
    untrack(() => (confirmDiscard = false));
  });
  const dirty = (id: EditTab): boolean => !!diff.tabs[id] || errorTabs.has(id);

  function addRow(kind: 'ports' | 'mounts' | 'env'): void {
    if (kind === 'ports') draft.ports.push(newPort());
    else if (kind === 'mounts') draft.mounts.push(newMount());
    else draft.env.push(newEnv());
  }
  function removeRow(kind: 'ports' | 'mounts' | 'env', id: number): void {
    if (kind === 'ports') draft.ports = draft.ports.filter((r) => r._id !== id);
    else if (kind === 'mounts') draft.mounts = draft.mounts.filter((r) => r._id !== id);
    else draft.env = draft.env.filter((r) => r._id !== id);
  }
  function tryClose(): void {
    if (applying) return;
    if (count > 0 && !confirmDiscard) {
      confirmDiscard = true;
      return;
    }
    onclose();
  }
  async function goReview(): Promise<void> {
    if (count === 0 || firstError) return;
    phase = 'review';
    applyError = '';
    await tick();
  }
  async function apply(): Promise<void> {
    if (applying || firstError) return;
    applying = true;
    applyError = '';
    try {
      onapply((await serverApi2.dockerEdit(serverId, container.id, toSpec(draft))).token);
    } catch (e) {
      applyError = errorText(e, t);
    } finally {
      applying = false;
    }
  }
</script>

{#snippet tag(recreate: boolean)}<span class="etag" class:re={recreate}>{t(recreate ? 'dockerEdit.tag.recreate' : 'dockerEdit.tag.live')}</span>{/snippet}

<Modal title={phase === 'review' ? t('dockerEdit.reviewTitle') : t('dockerEdit.title', { name: container.name })} wide onclose={tryClose}>
  <p class="dim mono" style="overflow-wrap:anywhere">{container.image}</p>
  {#if phase === 'edit'}
    <div class="row">
      {#each TABS as id (id)}<button class="chip" type="button" aria-pressed={tab === id} onclick={() => { tab = id; confirmDiscard = false; }}>{t(`dockerEdit.tabs.${id}`)}{#if dirty(id)} <span class="dirty-dot"></span>{/if}</button>{/each}
    </div>
    <div class="epane">
      {#if compose && tab !== 'main' && tab !== 'res'}<div class="banner">{t('dockerEdit.notice.composeOther', { project: compose.project, file: compose.file })}</div>
      {:else if compose}<div class="banner">{t('dockerEdit.notice.composeMain', { project: compose.project })}</div>{/if}

      {#if tab === 'main'}
        <div class="two">
          <label class="fl"><span>{t('dockerEdit.f.name')} {@render tag(false)}</span><span class="field" class:bad={!!errors.name}><input class="mono" autocomplete="off" spellcheck="false" disabled={!!compose} bind:value={draft.name} /></span></label>
          <label class="fl"><span>{t('dockerEdit.f.image')} {@render tag(true)}</span><span class="field" class:bad={!!errors.image}><input class="mono" autocomplete="off" spellcheck="false" disabled={!!compose} placeholder={t('dockerEdit.ph.image')} bind:value={draft.image} /></span></label>
        </div>
        <div class="two">
          <div class="fl"><span>{t('dockerEdit.f.restart')} {@render tag(false)}</span><CustomSelect value={draft.restart} options={restartOptions} onchange={(v) => (draft.restart = v as Draft['restart'])} /></div>
          {#if draft.restart === 'on-failure'}<label class="fl"><span>{t('dockerEdit.f.retries')}</span><span class="field" class:bad={!!errors.retries}><input class="mono" inputmode="numeric" autocomplete="off" bind:value={draft.retries} /></span><span class="dim">{t('dockerEdit.hint.retries')}</span></label>{/if}
        </div>
        <label class="fl"><span>{t('dockerEdit.f.cmd')} {@render tag(true)}</span><span class="field" class:bad={!!errors.cmd}><input class="mono" autocomplete="off" spellcheck="false" disabled={!!compose} placeholder={t('dockerEdit.ph.cmd')} bind:value={draft.cmd} /></span><span class="dim">{t('dockerEdit.hint.cmd')}</span></label>
      {:else if tab === 'ports'}
        {#if draft.ports.length === 0}<p class="dim">{t('dockerEdit.port.empty')}</p>{/if}
        {#each draft.ports as p (p._id)}
          <div class="erow" class:new={!origIds.has(p._id)}>
            <label class="fl"><span>{t('dockerEdit.port.ip')}</span><span class="field" class:bad={!!errors[`ports.${p._id}.ip`]}><input class="mono" autocomplete="off" spellcheck="false" disabled={!!compose} bind:value={p.ip} /></span></label>
            <label class="fl"><span>{t('dockerEdit.port.host')}</span><span class="field" class:bad={!!errors[`ports.${p._id}.host`]}><input class="mono" inputmode="numeric" autocomplete="off" disabled={!!compose} bind:value={p.host} /></span></label>
            <label class="fl"><span>{t('dockerEdit.port.cont')}</span><span class="field" class:bad={!!errors[`ports.${p._id}.cont`]}><input class="mono" inputmode="numeric" autocomplete="off" disabled={!!compose} bind:value={p.cont} /></span></label>
            <div class="fl"><span>{t('dockerEdit.port.proto')}</span><CustomSelect value={p.proto} options={protoOptions} onchange={(v) => (p.proto = v as 'tcp' | 'udp')} /></div>
            <button class="btn icon sm" type="button" aria-label={t('dockerEdit.port.remove')} disabled={!!compose} onclick={() => removeRow('ports', p._id)}><Icon name="x" size={14} /></button>
          </div>
        {/each}
        <div><button class="btn sm" type="button" disabled={!!compose} onclick={() => addRow('ports')}>{t('dockerEdit.port.add')}</button></div>
        <p class="dim">{t('dockerEdit.port.hint')}</p>
      {:else if tab === 'mounts'}
        {#if draft.mounts.length === 0}<p class="dim">{t('dockerEdit.mount.empty')}</p>{/if}
        {#each draft.mounts as m (m._id)}
          <div class="erow mounts" class:new={!origIds.has(m._id)}>
            <div class="fl"><span>{t('dockerEdit.mount.type')}</span><CustomSelect value={m.type} options={mountTypes} onchange={(v) => (m.type = v as 'bind' | 'volume')} /></div>
            <label class="fl"><span>{t(m.type === 'bind' ? 'dockerEdit.mount.srcBind' : 'dockerEdit.mount.srcVolume')}</span><span class="field" class:bad={!!errors[`mounts.${m._id}.src`]}><input class="mono" autocomplete="off" spellcheck="false" disabled={!!compose} bind:value={m.src} /></span></label>
            <label class="fl"><span>{t('dockerEdit.mount.dst')}</span><span class="field" class:bad={!!errors[`mounts.${m._id}.dst`]}><input class="mono" autocomplete="off" spellcheck="false" disabled={!!compose} bind:value={m.dst} /></span></label>
            <label class="ck" style="align-self:flex-end"><input type="checkbox" disabled={!!compose} bind:checked={m.ro} />{t('dockerEdit.mount.ro')}</label>
            <button class="btn icon sm" type="button" aria-label={t('dockerEdit.mount.remove')} disabled={!!compose} onclick={() => removeRow('mounts', m._id)}><Icon name="x" size={14} /></button>
          </div>
        {/each}
        <div><button class="btn sm" type="button" disabled={!!compose} onclick={() => addRow('mounts')}>{t('dockerEdit.mount.add')}</button></div>
        <p class="dim">{t('dockerEdit.mount.hint')}</p>
      {:else if tab === 'env'}
        {#if draft.env.length === 0}<p class="dim">{t('dockerEdit.env.empty')}</p>{/if}
        {#each draft.env as e (e._id)}
          {@const secret = isSecretName(e.k)}{@const shown = !!reveal[e._id]}
          <div class="erow env" class:new={!origIds.has(e._id)}>
            <label class="fl"><span>{t('dockerEdit.env.k')}</span><span class="field" class:bad={!!errors[`env.${e._id}.k`]}><input class="mono" autocomplete="off" spellcheck="false" disabled={!!compose} bind:value={e.k} /></span></label>
            <label class="fl"><span>{t('dockerEdit.env.v')}</span><span class="field"><input class="mono" type={secret && !shown ? 'password' : 'text'} autocomplete="off" spellcheck="false" disabled={!!compose} bind:value={e.v} />
              {#if secret}<button class="btn xs" type="button" style="width:auto;padding:0 6px" aria-pressed={shown} onclick={() => (reveal[e._id] = !shown)}>{t(shown ? 'dockerEdit.env.hide' : 'dockerEdit.env.show')}</button>{/if}</span></label>
            <button class="btn icon sm" type="button" aria-label={t('dockerEdit.env.remove')} disabled={!!compose} onclick={() => removeRow('env', e._id)}><Icon name="x" size={14} /></button>
          </div>
        {/each}
        <div><button class="btn sm" type="button" disabled={!!compose} onclick={() => addRow('env')}>{t('dockerEdit.env.add')}</button></div>
        <p class="dim">{t('dockerEdit.env.hint')}</p>
      {:else}
        <div class="two">
          <label class="fl"><span>{t('dockerEdit.f.mem')} {@render tag(false)}</span><span class="field" class:bad={!!errors.mem}><input class="mono" inputmode="numeric" autocomplete="off" placeholder={t('dockerEdit.ph.unlimited')} bind:value={draft.mem} /></span><span class="dim">{running ? t('dockerEdit.hint.mem', { used: fmtBytes(container.mem_used) }) : t('dockerEdit.hint.memIdle')}</span></label>
          <label class="fl"><span>{t('dockerEdit.f.cpus')} {@render tag(false)}</span><span class="field" class:bad={!!errors.cpus}><input class="mono" inputmode="decimal" autocomplete="off" placeholder={t('dockerEdit.ph.unlimited')} bind:value={draft.cpus} /></span><span class="dim">{t('dockerEdit.hint.cpus')}</span></label>
        </div>
      {/if}
    </div>
  {:else}
    {#if diff.recreate && running}<div class="warnbox warn">{t('dockerEdit.review.warn', { name: container.name })}</div>{/if}
    <ul class="ediff">
      {#each diff.changes as ch, i (i)}
        <li><span class="dim">{t(`dockerEdit.diff.${ch.field}`)}</span>
          {#if ch.kind === 'a'}<span class="ok-t">+ {ch.after}</span>{:else if ch.kind === 'd'}<span class="crit-t">{ch.before}</span>{:else}<span class="crit-t">{ch.before}</span> → <span class="ok-t">{ch.after}</span>{/if}</li>
      {/each}
    </ul>
    <div class="dim" style="display:grid;gap:3px">
      {#if diff.recreate}<span><b>1.</b> {t('dockerEdit.review.step1')}</span><span><b>2.</b> {t('dockerEdit.review.step2', { name: container.name })}</span><span><b>3.</b> {t('dockerEdit.review.step3')}</span><span><b>4.</b> {t('dockerEdit.review.step4')}</span>
      {:else}<span>{t('dockerEdit.review.stepsLive')}</span>{/if}
    </div>
    <p class="dim">{t('dockerEdit.review.audit')}</p>
  {/if}
  {#if applyError}<div class="err" role="alert">{applyError}</div>{/if}
  {#snippet foot()}
    {#if confirmDiscard}
      <span class="err-t">{t('dockerEdit.foot.discardText', { n: count })}</span><span class="spacer"></span>
      <button class="btn" type="button" onclick={onclose}>{t('dockerEdit.foot.discard')}</button>
      <button class="btn primary" type="button" onclick={() => (confirmDiscard = false)}>{t('dockerEdit.foot.keep')}</button>
    {:else if phase === 'edit'}
      <span class="dim">{#if count === 0}{t('dockerEdit.foot.none')}{:else}{t('dockerEdit.foot.count', { n: count })} · {diff.recreate ? t('dockerEdit.foot.recreate') : t('dockerEdit.foot.live')}{/if}</span>
      {#if firstError}<span class="err-t">{firstError}</span>{/if}<span class="spacer"></span>
      <button class="btn" type="button" onclick={tryClose}>{t('common.cancel')}</button>
      <button class="btn primary" type="button" disabled={count === 0 || !!firstError} onclick={goReview}>{t('dockerEdit.foot.next')}</button>
    {:else}
      <span class="spacer"></span>
      <button class="btn" type="button" disabled={applying} onclick={() => (phase = 'edit')}>{t('dockerEdit.review.back')}</button>
      <button class="btn {diff.recreate ? 'danger' : 'primary'}" type="button" disabled={applying} onclick={apply}>{applying ? t('dockerEdit.review.busy') : t(diff.recreate ? 'dockerEdit.review.recreateApply' : 'dockerEdit.review.apply')}</button>
    {/if}
  {/snippet}
</Modal>
