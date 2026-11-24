import { getPref, setPref } from './prefs';
import ru from './locales/ru.json';
import en from './locales/en.json';

export type Locale = 'ru' | 'en';

const dict: Record<Locale, Record<string, string>> = { ru, en };
const KEY = 'linager.locale';

function initial(): Locale {
  try {
    const saved = getPref(KEY);
    if (saved === 'ru' || saved === 'en') return saved;
  } catch {
    // no storage
  }
  return navigator.language.toLowerCase().startsWith('ru') ? 'ru' : 'en';
}

export const i18n = $state<{ locale: Locale }>({ locale: initial() });

export function setLocale(l: Locale): void {
  i18n.locale = l;
  document.documentElement.lang = l;
  setPref(KEY, l);
}

/** Translate a key; {name} placeholders are filled from vars. Reads i18n.locale, so a template re-renders on a change. */
export function t(key: string, vars?: Record<string, string | number>): string {
  const raw = dict[i18n.locale][key] ?? dict.ru[key] ?? key;
  return vars ? raw.replace(/\{(\w+)\}/g, (m, n: string) => (n in vars ? String(vars[n]) : m)) : raw;
}

document.documentElement.lang = i18n.locale;
