import { derived, get, writable } from 'svelte/store';
import ru from './locales/ru.json';
import en from './locales/en.json';
import ruServer from './locales/ru.server.json';
import enServer from './locales/en.server.json';
import ruTerm from './locales/ru.term.json';
import enTerm from './locales/en.term.json';
import ruMgmt from './locales/ru.mgmt.json';
import enMgmt from './locales/en.mgmt.json';
import ruFiles from './locales/ru.files.json';
import enFiles from './locales/en.files.json';
import ruNet from './locales/ru.net.json';
import enNet from './locales/en.net.json';
import ruFw from './locales/ru.fw.json';
import enFw from './locales/en.fw.json';
import ruUpd from './locales/ru.upd.json';
import enUpd from './locales/en.upd.json';
import ruAudit from './locales/ru.audit.json';
import ruSettings from './locales/ru.settings.json';
import ruAccess from './locales/ru.access.json';
import ruConn from './locales/ru.conn.json';
import ruSec from './locales/ru.sec.json';
import ruDisk from './locales/ru.disk.json';
import enAudit from './locales/en.audit.json';
import enSettings from './locales/en.settings.json';
import enAccess from './locales/en.access.json';
import enConn from './locales/en.conn.json';
import enSec from './locales/en.sec.json';
import enDisk from './locales/en.disk.json';
import ruUsers from './locales/ru.users.json';
import enUsers from './locales/en.users.json';
import ruMobA from './locales/ru.mobA.json';
import enMobA from './locales/en.mobA.json';
import ruMobB from './locales/ru.mobB.json';
import enMobB from './locales/en.mobB.json';
import ruMobC from './locales/ru.mobC.json';
import enMobC from './locales/en.mobC.json';
import ruMobD from './locales/ru.mobD.json';
import enMobD from './locales/en.mobD.json';
import ruMobile from './locales/ru.mobile.json';
import enMobile from './locales/en.mobile.json';
import ruAddsrv from './locales/ru.addsrv.json';
import ruDesk from './locales/ru.desk.json';
import enDesk from './locales/en.desk.json';
import ruWin from './locales/ru.winmods.json';
import enWin from './locales/en.winmods.json';
import ruAssist from './locales/ru.assist.json';
import ruPult from './locales/ru.pult.json';
import enPult from './locales/en.pult.json';
import enAssist from './locales/en.assist.json';
import enAddsrv from './locales/en.addsrv.json';
import ruKeys from './locales/ru.keys.json';
import enKeys from './locales/en.keys.json';

export type Locale = 'ru' | 'en';
export type Vars = Record<string, string | number>;

const dictionaries: Record<Locale, Record<string, string>> = {
  ru: { ...ru, ...ruServer, ...ruTerm, ...ruMgmt, ...ruFiles, ...ruNet, ...ruFw, ...ruUpd, ...ruAudit, ...ruSettings, ...ruAccess, ...ruConn, ...ruSec, ...ruDisk, ...ruUsers, ...ruMobile, ...ruMobA, ...ruMobB, ...ruMobC, ...ruMobD, ...ruAddsrv, ...ruDesk, ...ruWin, ...ruAssist, ...ruPult, ...ruKeys },
  en: { ...en, ...enServer, ...enTerm, ...enMgmt, ...enFiles, ...enNet, ...enFw, ...enUpd, ...enAudit, ...enSettings, ...enAccess, ...enConn, ...enSec, ...enDisk, ...enUsers, ...enMobile, ...enMobA, ...enMobB, ...enMobC, ...enMobD, ...enAddsrv, ...enDesk, ...enWin, ...enAssist, ...enPult, ...enKeys },
};
const STORAGE_KEY = 'linager.lang';
const DEFAULT_LOCALE: Locale = 'ru';

function readStored(): Locale {
  try {
    const value = localStorage.getItem(STORAGE_KEY);
    if (value === 'ru' || value === 'en') return value;
  } catch {
    // storage unavailable (private mode, blocked) - fall back to default
  }
  return DEFAULT_LOCALE;
}

export const locale = writable<Locale>(readStored());

export function setLocale(next: Locale): void {
  locale.set(next);
  try {
    localStorage.setItem(STORAGE_KEY, next);
  } catch {
    // ignore storage errors
  }
  document.documentElement.lang = next;
}

export function t(key: string, current: Locale = get(locale), vars?: Vars): string {
  const raw = dictionaries[current][key] ?? dictionaries[DEFAULT_LOCALE][key] ?? key;
  if (!vars) return raw;
  return raw.replace(/\{(\w+)\}/g, (match: string, name: string) =>
    name in vars ? String(vars[name]) : match,
  );
}

// Reactive translator for templates: `{$tr('key')}` re-renders on locale change.
export const tr = derived(locale, ($locale) => (key: string, vars?: Vars) => t(key, $locale, vars));

document.documentElement.lang = get(locale);
