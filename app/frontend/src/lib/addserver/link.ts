import { t } from '../i18n.svelte';

/** "5 h 12 min", "3 d 4 h", "7 min" for a number of seconds left (always at least a minute). */
export function fmtIn(seconds: number): string {
  const total = Math.max(Math.round(seconds / 60), 1);
  const d = Math.floor(total / 1440);
  const h = Math.floor((total % 1440) / 60);
  const m = total % 60;
  if (d > 0) return t('asv.link.dur.dh', { d, h });
  if (h > 0) return t('asv.link.dur.hm', { h, m });
  return t('asv.link.dur.m', { m });
}

/** Single-quote a value for a POSIX shell. */
function shq(s: string): string {
  return `'${s.replace(/'/g, `'\\''`)}'`;
}

/**
 * The text of the install script shown to the administrator, built here from the command the server made:
 * the address (host:port) and the pinned key are the ones of the command, the token is the one of the new link.
 * The script the server really serves at the link also checks the OS and the checksum of the binary.
 */
export function joinScriptText(token: string, command: string, fingerprint: string): string {
  const pin = /--pinnedpubkey\s+'([^']*)'/.exec(command)?.[1] ?? '';
  const server = /https:\/\/([^/'\s]+)\/v1\/join\//.exec(command)?.[1] ?? '';
  return [
    '#!/bin/sh',
    '# Linager: install the agent by a link made in the application.',
    '# Abridged: the script served at the link also checks the OS and the checksum of the binary.',
    'set -eu',
    '',
    `SERVER=${shq(server)}`,
    `FP=${shq(fingerprint)}`,
    `PIN=${shq(pin)}`,
    `TOKEN=${shq(token)}`,
    '',
    '[ "$(id -u)" -eq 0 ] || { echo "run as root" >&2; exit 1; }',
    'arch=$(dpkg --print-architecture)   # amd64 or arm64',
    'tmp=$(mktemp -d); trap \'rm -rf "$tmp"\' EXIT',
    'wget -nv --no-check-certificate --pinnedpubkey "$PIN" -O "$tmp/linager" "https://$SERVER/v1/join/$TOKEN/binary?arch=$arch"',
    'chmod 0755 "$tmp/linager"',
    '"$tmp/linager" install agent --no-tui --server "$SERVER" --fingerprint "$FP" --join-token "$TOKEN"',
    '',
  ].join('\n');
}
