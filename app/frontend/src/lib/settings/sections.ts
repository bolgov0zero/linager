// The sections of the Settings window, in the order of the list. An operator and an auditor only have the application
// and their own profile; the rest is for an administrator.
export type SettingsSection = 'app' | 'profile' | 'users' | 'access' | 'connection' | 'updates' | 'audit' | 'snippets' | 'folders' | 'tags';

export const SETTINGS_SECTIONS: { id: SettingsSection; admin: boolean }[] = [
  { id: 'app', admin: false },
  { id: 'profile', admin: false },
  { id: 'users', admin: true },
  { id: 'snippets', admin: true },
  { id: 'folders', admin: true },
  { id: 'tags', admin: true },
  { id: 'access', admin: true },
  { id: 'connection', admin: true },
  { id: 'updates', admin: true },
  { id: 'audit', admin: true },
];

export function isSection(v: string): v is SettingsSection {
  return SETTINGS_SECTIONS.some((s) => s.id === v);
}
