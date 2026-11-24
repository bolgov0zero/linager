import { get, writable } from 'svelte/store';
import { assistantApi, type AssistSettings } from '../api';
import { getPref, setPref } from '../prefs';

/** The picture settings of every computer whose Assistant is on screen, as last read from the server. */
export const assistSettings = writable<Record<string, AssistSettings>>({});

export function setAssistSettings(serverId: string, settings: AssistSettings): void {
  assistSettings.update((all) => ({ ...all, [serverId]: settings }));
}

/** Reads the settings again; a failure leaves what is known. */
export async function refreshAssistSettings(serverId: string): Promise<void> {
  try {
    setAssistSettings(serverId, await assistantApi.settings(serverId));
  } catch {
    // the limits are just not shown
  }
}

const INFO_KEY = 'linager.assist.info';

/** Whether the line under the screen (resolution, colours, frames, traffic) is shown. Off by default; one choice for every Assistant. */
export const assistInfoShown = writable<boolean>(getPref(INFO_KEY) === '1');

export function toggleAssistInfo(): void {
  const next = !get(assistInfoShown);
  assistInfoShown.set(next);
  setPref(INFO_KEY, next ? '1' : '0');
}
