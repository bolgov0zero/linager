import { native, NativeError } from './native';

// What was cut or copied in the file manager; shared by the files tab and the files window of one page.
export interface Clip {
  mode: 'cut' | 'copy';
  serverId: string;
  dir: string;
  names: string[];
}
export const clipboard = $state<{ clip: Clip | null }>({ clip: null });

/** Downloads a file of the server. The window of the application does not handle a download link, so the application
 *  saves it itself (a native dialog); in a browser (development) the link is followed. Returns the saved path, or
 *  null when it was cancelled. */
export async function startDownload(url: string, name: string): Promise<string | null> {
  try {
    const r = await native.download(url, name);
    return r.cancelled ? null : (r.saved ?? null);
  } catch (e) {
    if (!(e instanceof NativeError && e.status === 501)) throw e;
  }
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.rel = 'noopener';
  document.body.appendChild(a);
  a.click();
  a.remove();
  return null;
}
