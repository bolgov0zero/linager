import type { AssistSettings } from '../api';

/** The ready-made picture settings of the Assistant. */
export const PRESETS: Record<'eco' | 'normal' | 'hq', AssistSettings> = {
  eco: { colors: '256', fps: 8, mbit: 1, nolimit: false, auto: true, show_cursor: false },
  normal: { colors: '24', fps: 15, mbit: 4, nolimit: false, auto: true, show_cursor: false },
  hq: { colors: '24', fps: 30, mbit: 12, nolimit: false, auto: true, show_cursor: false },
};
export const PRESET_IDS = ['eco', 'normal', 'hq'] as const;
export const COLORS = ['24', '16', '256', 'gray'] as const;

/** The preset the settings equal (the pointer does not count), or 'custom'. */
export function presetOf(s: AssistSettings): 'eco' | 'normal' | 'hq' | 'custom' {
  return (
    PRESET_IDS.find((id) => {
      const p = PRESETS[id];
      return p.colors === s.colors && p.fps === s.fps && p.nolimit === s.nolimit && p.auto === s.auto && (s.nolimit || p.mbit === s.mbit);
    }) ?? 'custom'
  );
}

/** A rough guess of the traffic of a busy screen in Mbit/s; the real figure is on the line under the picture. */
export function estimateMbit(s: AssistSettings): number {
  const k = { '24': 1, '16': 0.66, '256': 0.35, gray: 0.3 }[s.colors];
  return Math.min(s.nolimit ? 99 : s.mbit, Math.round(s.fps * 0.55 * k * 10) / 10);
}
