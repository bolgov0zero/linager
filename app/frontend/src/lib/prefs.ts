// The choices of the person (theme, language, layout of the dashboard, section). A page of the application lives on a
// random address, so the storage of the web view is lost between starts: the choices are kept by the application in
// a file (/app/prefs). localStorage stays as the fallback (a browser during development, an old application).
let cache: Record<string, string> = {};

/** Reads the saved choices; called once, before the interface is built. */
export async function loadPrefs(): Promise<void> {
  try {
    const res = await fetch('app/prefs');
    if (res.ok) cache = (await res.json()) as Record<string, string>;
  } catch {
    // no application behind the page: localStorage only
  }
}

export function getPref(key: string): string | null {
  if (key in cache) return cache[key];
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

export function setPref(key: string, value: string): void {
  cache[key] = value;
  try {
    localStorage.setItem(key, value);
  } catch {
    // no storage
  }
  void fetch('app/prefs', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ [key]: value }) }).catch(() => undefined);
}
