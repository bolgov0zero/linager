<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Session } from '../api';
  import { t } from '../i18n.svelte';
  import AppSection from '../settings/AppSection.svelte';
  import FoldersSection from '../settings/FoldersSection.svelte';
  import TagsSection from '../settings/TagsSection.svelte';
  import ProfileSection from '../settings/ProfileSection.svelte';
  import { isSection, SETTINGS_SECTIONS, type SettingsSection } from '../settings/sections';
  import SnippetsSection from '../settings/SnippetsSection.svelte';
  import AccessSection from '../settings/AccessSection.svelte';
  import AuditSection from '../settings/AuditSection.svelte';
  import ConnectionSection from '../settings/ConnectionSection.svelte';
  import UpdatesSection from '../settings/UpdatesSection.svelte';
  import UsersSection from '../settings/UsersSection.svelte';

  // Settings in a window of their own: the sections of the list on the left, the chosen one on the right.
  let { session: first, start = '' }: { session: Session; start?: string } = $props();

  // svelte-ignore state_referenced_locally
  let session = $state<Session>(first);
  // svelte-ignore state_referenced_locally
  let section = $state<SettingsSection>(isSection(start) ? start : 'app');
  let lost = $state(false);

  const isAdmin = $derived((session.role ?? 'admin') === 'admin');
  const sections = $derived(SETTINGS_SECTIONS.filter((s) => isAdmin || !s.admin));
  const current = $derived(sections.some((s) => s.id === section) ? section : 'app');

  async function refresh(): Promise<void> {
    try {
      session = await api.me();
    } catch {
      // the session may have ended: the next call says so
    }
  }
  onMount(() => {
    document.title = `${t('st.settings.window')} · Linager`;
  });
</script>

<div class="set">
  <nav class="set-nav" aria-label={t('st.settings.nav')}>
    <h2>{t('st.settings.title')}</h2>
    {#each sections as s (s.id)}
      <button class="sn" type="button" aria-current={current === s.id} onclick={() => (section = s.id)}>{t(`st.settings.section.${s.id}`)}</button>
    {/each}
    <div class="set-foot">w/ ❤️ by bolgov0zero</div>
  </nav>
  <div class="set-body">
    {#if lost}<div class="err" role="alert" style="margin-bottom:12px">{t('st.settings.lost')}</div>{/if}
    {#key current}
      {#if current === 'app'}<AppSection />
      {:else if current === 'profile'}<ProfileSection {session} onrefresh={refresh} onauthlost={() => (lost = true)} />
      {:else if current === 'users'}<UsersSection onauthlost={() => (lost = true)} />
      {:else if current === 'access'}<AccessSection onauthlost={() => (lost = true)} />
      {:else if current === 'connection'}<ConnectionSection onauthlost={() => (lost = true)} />
      {:else if current === 'updates'}<UpdatesSection />
      {:else if current === 'audit'}<AuditSection />
      {:else if current === 'snippets'}<SnippetsSection onauthlost={() => (lost = true)} />
      {:else if current === 'folders'}<FoldersSection onauthlost={() => (lost = true)} />
      {:else if current === 'tags'}<TagsSection onauthlost={() => (lost = true)} />{/if}
    {/key}
  </div>
</div>
