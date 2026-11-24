<script lang="ts">
  import { api, type Group, type Role, type ServerCard, type UserAccess } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { t } from '../i18n.svelte';
  import AccessModulesModal from './AccessModulesModal.svelte';
  import AccessServersModal from './AccessServersModal.svelte';

  let { role = $bindable(), access = $bindable() }: { role: Role; access: UserAccess } = $props();

  let serversOpen = $state(false);
  let modulesOpen = $state(false);
  let servers = $state<ServerCard[]>([]);
  let groups = $state<Group[]>([]);
  let loadError = $state(false);

  const roles = $derived((['admin', 'operator', 'auditor'] as Role[]).map((r) => ({ v: r, t: t(`st.users.role.${r}`) })));
  const serverCount = $derived(new Set([...access.servers, ...servers.filter((s) => s.group_id != null && access.groups.includes(s.group_id)).map((s) => s.id)]).size || access.servers.length);

  async function openServers(): Promise<void> {
    loadError = false;
    try {
      const [d, g] = await Promise.all([api.dashboard(), api.groups()]);
      servers = d.items;
      groups = g;
      serversOpen = true;
    } catch {
      loadError = true;
    }
  }
</script>

<div class="mdl-stack">
  <div class="fl"><span>{t('st.users.role')}</span><CustomSelect value={role} options={roles} onchange={(v) => (role = v as Role)} /></div>
  <p class="dim">{t(`st.users.roleHint.${role}`)}</p>
  {#if role !== 'admin'}
    <div class="row">
      <button class="btn" type="button" onclick={() => void openServers()}>{t('st.acc.servers.button')} <span class="dim">{serverCount}</span></button>
      {#if role === 'operator'}<button class="btn" type="button" onclick={() => (modulesOpen = true)}>{t('st.acc.modules.button')} <span class="dim">{access.modules.length}</span></button>{/if}
    </div>
    {#if loadError}<div class="err" role="alert">{t('common.genericError')}</div>{/if}
  {/if}
</div>

{#if serversOpen}
  <AccessServersModal {servers} {groups} selectedServers={access.servers} selectedGroups={access.groups} onsave={(s, g) => { access = { ...access, servers: s, groups: g }; serversOpen = false; }} onclose={() => (serversOpen = false)} />
{/if}
{#if modulesOpen}
  <AccessModulesModal selected={access.modules} onsave={(m) => { access = { ...access, modules: m }; modulesOpen = false; }} onclose={() => (modulesOpen = false)} />
{/if}
