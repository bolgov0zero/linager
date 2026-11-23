import { PIXELART } from './pixelart';

/**
 * The names the interface uses for its icons, and the Pixelarticons icon each one is drawn with. A name that is not here is
 * looked up in Pixelarticons itself, so a new icon only needs its path in pixelart.ts.
 */
const ALIAS: Record<string, string> = {
  // buttons
  more: 'more-horizontal',
  back: 'arrow-left',
  x: 'close',
  cards: 'grid-2x2-2',
  gear: 'settings-cog',
  next: 'chevron-right',
  chev: 'chevron-down',
  audit: 'file-text',
  reboot: 'power',
  servers: 'server',
  tasks: 'analytics',
  menu: 'menu',
  paste: 'clipboard',
  window: 'window-frame',
  bolt: 'zap',
  wrap: 'text-wrap',
  warn: 'warning-diamond',
  // the connections (the Console, the cards)
  desktop: 'monitor',
  assistant: 'mouse',
  // the tabs of a server page
  overview: 'grid-2x2-2',
  metrics: 'chart-bar-big',
  manage: 'sliders',
  services: 'bulletlist',
  processes: 'cpu',
  programs: 'window-frame',
  logs: 'article',
  updates: 'download',
  files: 'folder',
  disk: 'database',
  network: 'globe',
  firewall: 'fire',
  security: 'shield',
  docker: 'docker',
};

const FALLBACK = 'more-horizontal';

/** The path data of an icon by the name the interface uses. */
export function iconPath(name: string): string {
  return PIXELART[ALIAS[name] ?? name] ?? PIXELART[FALLBACK];
}
