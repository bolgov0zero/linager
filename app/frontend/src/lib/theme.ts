export type Theme = 'dark' | 'light';

const KEY = 'linager.theme';

export function currentTheme(): Theme {
  return document.documentElement.dataset.theme === 'light' ? 'light' : 'dark';
}

export function setTheme(t: Theme): void {
  document.documentElement.dataset.theme = t;
  try {
    localStorage.setItem(KEY, t);
  } catch {
    // no storage: the choice lasts until the window closes
  }
}

/** Dark is the default (the mockup is dark-first); a saved choice wins, then the system setting. */
export function initTheme(): void {
  let t: Theme = 'dark';
  try {
    const saved = localStorage.getItem(KEY);
    if (saved === 'light' || saved === 'dark') t = saved;
    else if (matchMedia('(prefers-color-scheme: light)').matches) t = 'light';
  } catch {
    // keep the default
  }
  document.documentElement.dataset.theme = t;
}
