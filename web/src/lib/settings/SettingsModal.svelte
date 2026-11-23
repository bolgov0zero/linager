<script lang="ts">
  import type { Session } from '../../api';
  import { tr } from '../../i18n';
  import Modal from '../Modal.svelte';
  import AuditPage from '../AuditPage.svelte';
  import UpdatesPage from '../UpdatesPage.svelte';
  import ProfileSection from './ProfileSection.svelte';
  import SnippetsSection from './SnippetsSection.svelte';
  import GroupsSection from './GroupsSection.svelte';
  import UsersSection from './UsersSection.svelte';
  import AccessSection from './AccessSection.svelte';
  import ConnectionSection from './ConnectionSection.svelte';

  import { SETTINGS_SECTIONS, type SettingsSection } from './sections';

  let {
    section,
    list = false,
    session,
    updatesCount,
    onsection,
    onclose,
    onauthlost,
    onrefresh,
    onupdateschanged,
  }: {
    section: SettingsSection;
    /** Phone: start with the list of sections instead of a section. */
    list?: boolean;
    session: Session;
    updatesCount: number;
    onsection: (s: SettingsSection) => void;
    onclose: () => void;
    onauthlost: () => void;
    onrefresh: () => Promise<void>;
    onupdateschanged: () => void;
  } = $props();

  /** An operator and an auditor only have their profile. */
  const isAdmin = $derived((session.role ?? 'admin') === 'admin');
  // Phone: the list of sections and a section are two screens, "back" returns to the list.
  // svelte-ignore state_referenced_locally
  let showList = $state(list);
</script>

<Modal title={isAdmin ? $tr('settings.title') : $tr('app.profile')} labelId="settings-title" {onclose} wide={isAdmin}>
  <div class="settings" class:single={!isAdmin} class:list={showList && isAdmin}>
    {#if isAdmin}
    <nav class="side" aria-label={$tr('settings.nav')}>
      {#each SETTINGS_SECTIONS as s (s)}
        <button class="nav" class:active={section === s} type="button" aria-current={section === s ? 'page' : undefined} onclick={() => { showList = false; onsection(s); }}>
          {$tr(`settings.section.${s}`)}
          {#if s === 'updates' && updatesCount > 0}<span class="badge">{updatesCount}</span>{/if}
        </button>
      {/each}
    </nav>
    {/if}
    <div class="content">
      {#if isAdmin}
        <button class="btn back" type="button" onclick={() => (showList = true)}>{$tr('settings.back')}</button>
      {/if}
      {#if section === 'profile'}
        <ProfileSection {session} {onrefresh} {onauthlost} />
      {:else if section === 'users'}
        <UsersSection {onauthlost} />
      {:else if section === 'groups'}
        <GroupsSection {onauthlost} />
      {:else if section === 'snippets'}
        <SnippetsSection {onauthlost} />
      {:else if section === 'access'}
        <AccessSection {onauthlost} />
      {:else if section === 'connection'}
        <ConnectionSection {onauthlost} />
      {:else if section === 'updates'}
        <UpdatesPage embedded onchanged={onupdateschanged} />
      {:else}
        <AuditPage embedded />
      {/if}
    </div>
  </div>
</Modal>

<style>
  .settings {
    display: grid;
    grid-template-columns: 171px minmax(0, 1fr);
    gap: 18px;
    min-height: min(576px, calc(100dvh - 162px));
  }

  .settings.single {
    display: block;
    min-height: 0;
  }

  .side {
    display: flex;
    flex-direction: column;
    gap: 5px;
    align-self: start;
  }

  .nav {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 7px;
    padding: 7px 11px;
    text-align: left;
    background: var(--surface);
    color: var(--ink);
    border: 2px solid var(--line);
    border-radius: 0;
    font-family: var(--font-display);
    font-size: 13px;
    cursor: pointer;
    box-shadow: 3px 3px 0 var(--shade);
  }

  .nav.active {
    background: var(--ink);
    color: var(--surface);
  }

  .badge {
    padding: 0 5px;
    border: 2px solid currentColor;
    font-size: 11px;
  }

  .content {
    min-width: 0;
    max-height: calc(100dvh - 171px);
    overflow-y: auto;
    padding: 2px 4px 7px 2px;
  }

  .back {
    display: none;
  }

  /* Phone: a list of sections, then one section on its own screen with a way back. */
  @media (max-width: 720px) {
    .settings {
      grid-template-columns: minmax(0, 1fr);
      grid-auto-rows: max-content;
      gap: 11px;
      min-height: 0;
    }

    .settings:not(.list) .side {
      display: none;
    }

    .settings.list .content {
      display: none;
    }

    .nav {
      min-height: 54px;
      font-size: 16px;
    }

    .content {
      max-height: none;
      padding: 0;
    }

    .back {
      display: inline-flex;
      margin-bottom: 11px;
    }
  }
</style>
