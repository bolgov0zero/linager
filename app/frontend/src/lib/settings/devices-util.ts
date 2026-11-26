// Settings, "Devices": how a device is shown. The platform is what the application said about itself (X-Linager-Client).
import type { DeviceRow } from '../api';

const PLATFORMS = new Set(['ios', 'android', 'windows', 'macos', 'linux']);

/** The key of the name of a platform; an unknown one is "other". */
export const platformKey = (p: string): string => `st.dev.platform.${PLATFORMS.has(p) ? p : 'other'}`;

/** A phone or a tablet (it gets the phone icon). */
export const isMobile = (p: string): boolean => p === 'ios' || p === 'android';

/** What the person knows the device by: the name they gave it, else the model (the name of a computer, "iPhone18,1"), else the platform. */
export function deviceTitle(d: Pick<DeviceRow, 'label' | 'model' | 'platform'>, platformName: string): string {
  return d.label.trim() || d.model.trim() || platformName;
}

/** The version of the application with the system: "1.2.3 · iOS 27.1" (what is not known is left out). */
export function versions(d: Pick<DeviceRow, 'app_version' | 'os_version'>): string {
  return [d.app_version, d.os_version].filter((v) => v.trim()).join(' · ');
}

/** Devices that wait for a decision. */
export const waiting = (list: readonly DeviceRow[]): DeviceRow[] => list.filter((d) => d.status === 'pending');
