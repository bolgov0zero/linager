// The sections of the Settings window, in the order of the list. An operator and an auditor only have the application
// and their own profile; the rest is for an administrator.
export type SettingsSection = 'app' | 'profile' | 'users' | 'access' | 'connection' | 'updates' | 'audit' | 'snippets' | 'folders' | 'tags';

export const SETTINGS_SECTIONS: { id: SettingsSection; admin: boolean; icon: string }[] = [
  { id: 'app', icon: 'sliders-horizontal', admin: false },
  { id: 'profile', icon: 'user', admin: false },
  { id: 'users', icon: 'users', admin: true },
  { id: 'snippets', icon: 'file-code', admin: true },
  { id: 'folders', icon: 'folders', admin: true },
  { id: 'tags', icon: 'tag', admin: true },
  { id: 'access', icon: 'shield-check', admin: true },
  { id: 'connection', icon: 'network', admin: true },
  { id: 'updates', icon: 'download', admin: true },
  { id: 'audit', icon: 'scroll-text', admin: true },
];

export function isSection(v: string): v is SettingsSection {
  return SETTINGS_SECTIONS.some((s) => s.id === v);
}
