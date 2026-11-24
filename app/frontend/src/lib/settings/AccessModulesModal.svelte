<script lang="ts" module>
  /** The agent modules an operator can get, and the ones that give the access of root on the host. */
  export const MODULES = ['terminal', 'desktop', 'assistant', 'files', 'services', 'apt', 'processes', 'logs', 'disk', 'network', 'firewall', 'security', 'users', 'docker'] as const;
  export const ROOT_MODULES: readonly string[] = ['terminal', 'desktop', 'assistant', 'files', 'network', 'firewall', 'security', 'users', 'docker'];
</script>

<script lang="ts">
  import { untrack } from 'svelte';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';

  let { selected, onsave, onclose }: { selected: string[]; onsave: (modules: string[]) => void; onclose: () => void } = $props();

  let picked = $state<string[]>(untrack(() => [...selected]));

  // the groups of the navigation of the server page
  const GROUPS: { key: string; modules: string[] }[] = [
    { key: 'general', modules: ['terminal', 'desktop', 'assistant'] },
    { key: 'system', modules: ['services', 'processes', 'logs', 'apt'] },
    { key: 'fs', modules: ['files', 'disk'] },
    { key: 'net', modules: ['network', 'firewall', 'security', 'users'] },
    { key: 'apps', modules: ['docker'] },
  ];
  const LABEL: Record<string, string> = { terminal: 'sp.terminal', assistant: 'sp.assistant', services: 'sp.services', processes: 'sp.processes', logs: 'sp.logs', apt: 'sp.updates', files: 'sp.files', disk: 'sp.disk', network: 'sp.network', firewall: 'sp.firewall', security: 'sp.security', users: 'sp.users', docker: 'Docker' };
  const label = (m: string): string => (m === 'desktop' ? 'RDP' : LABEL[m]?.startsWith('sp.') ? t(LABEL[m]) : (LABEL[m] ?? m));
  const rootPicked = $derived(picked.filter((m) => ROOT_MODULES.includes(m)));
  const toggle = (m: string): void => {
    picked = picked.includes(m) ? picked.filter((x) => x !== m) : [...picked, m];
  };
</script>

<Modal title={t('st.acc.modules.title')} {onclose}>
  <p class="dim">{t('st.acc.modules.hint')}</p>
  <div class="mod-list">
    {#each GROUPS as g (g.key)}
      <div class="mod-grp">{t(`sp.group.${g.key}`)}</div>
      {#each g.modules as m (m)}
        <label class="mod-row">
          <input type="checkbox" checked={picked.includes(m)} onchange={() => toggle(m)} />
          <span class="txt"><b>{label(m)}</b><small class="dim">{t(`st.acc.mod.${m}`)}</small></span>
          {#if ROOT_MODULES.includes(m)}<span class="badge b-warn">{t('st.acc.modules.root')}</span>{/if}
        </label>
      {/each}
    {/each}
  </div>
  {#if rootPicked.length > 0}<p class="warnbox warn" role="status">{t('st.acc.modules.rootWarn')}</p>{/if}
  {#snippet foot()}
    <span class="dim">{t('st.acc.modules.total', { n: picked.length })}</span>
    <span class="spacer"></span>
    <button class="btn" type="button" onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" onclick={() => onsave(MODULES.filter((m) => picked.includes(m)))}>{t('st.acc.apply')}</button>
  {/snippet}
</Modal>
