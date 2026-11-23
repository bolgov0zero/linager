/** Known audit action keys. Each one has an `audit.action.<key>` translation; other keys are shown raw. */
export const KNOWN_ACTIONS: ReadonlySet<string> = new Set([
  'auth.login', 'auth.logout', 'auth.totp.enable', 'auth.totp.disable', 'auth.passkey.add', 'auth.passkey.delete', 'auth.recovery.regenerate',
  'user.create', 'user.delete', 'user.password.change', 'user.password.reset', 'user.2fa.reset',
  'security.session.terminate',
  'hostuser.add', 'hostuser.update', 'hostuser.delete', 'hostuser.password', 'hostuser.kill',
  'snippet.create', 'snippet.update', 'snippet.delete', 'snippet.order', 'snippet.run',
  'enroll.approve', 'enroll.reject', 'server.delete', 'server.reboot',
  'service.start', 'service.stop', 'service.restart', 'service.enable', 'service.disable', 'service.ignore', 'service.unignore',
  'process.kill',
  'files.write', 'files.mkdir', 'files.create', 'files.delete', 'files.upload', 'files.download', 'files.rename', 'files.chmod', 'files.chown', 'files.copy', 'files.move', 'files.zip',
  'network.apply', 'network.config.apply', 'network.confirm', 'network.rollback', 'network.draft.save', 'network.draft.reset',
  'firewall.apply', 'firewall.confirm', 'firewall.rollback', 'firewall.draft.save', 'firewall.draft.reset',
  'docker.container.start', 'docker.container.stop', 'docker.container.restart', 'docker.container.remove', 'docker.container.edit',
  'docker.image.remove', 'docker.ignore', 'docker.unignore', 'apt.ignore', 'apt.unignore',
  'update.check', 'update.channel', 'update.prefetch', 'update.cache.prune', 'update.server', 'update.agents',
  'stream.open', 'stream.close',
  'desktop.open', 'desktop.close', 'desktop.enable', 'desktop.disable',
  'assistant.open', 'assistant.close', 'assistant.settings', 'assistant.sas',
  'metrics.export',
  'dashboard.add', 'dashboard.remove', 'dashboard.reorder',
  'group.create', 'group.rename', 'group.delete', 'group.assign',
  'access.update', 'access.retention', 'access.reset',
  'assistant.type', 'disk.scan', 'disk.clean', 'docker.check', 'group.reorder', 'join.create', 'join.revoke', 'network.dns', 'server.note', 'user.access', 'user.password_reset', 'security.fail2ban.ban', 'security.fail2ban.unban', 'security.fail2ban.settings', 'security.key.remove', 'ssh.check', 'ssh.install', 'enroll.join', 'win.users.add', 'win.users.update', 'win.users.passwd', 'win.users.delete', 'win.users.logoff', 'win.network.apply', 'win.network.rollback',
]);

export interface Option {
  value: string;
  key: string;
}

/** Action group filter. The value is sent as the `action` parameter. */
export const ACTION_GROUPS: Option[] = [
  { value: '', key: 'audit.filter.all' },
  { value: 'auth.', key: 'audit.group.auth' },
  { value: 'user.', key: 'audit.group.users' },
  { value: 'snippet.', key: 'audit.group.snippets' },
  { value: 'security.', key: 'audit.group.security' },
  { value: 'hostuser.', key: 'audit.group.hostusers' },
  { value: 'enroll.,server.', key: 'audit.group.servers' },
  { value: 'network.', key: 'audit.group.network' },
  { value: 'firewall.', key: 'audit.group.firewall' },
  { value: 'files.', key: 'audit.group.files' },
  { value: 'service.', key: 'audit.group.services' },
  { value: 'stream.', key: 'audit.group.stream' },
  { value: 'desktop.', key: 'audit.group.desktop' },
  { value: 'assistant.', key: 'audit.group.assistant' },
  { value: 'update.', key: 'audit.group.updates' },
  { value: 'docker.', key: 'audit.group.docker' },
  { value: 'process.', key: 'audit.group.process' },
  { value: 'metrics.', key: 'audit.group.metrics' },
  { value: 'dashboard.', key: 'audit.group.dashboard' },
  { value: 'access.', key: 'audit.group.access' },
];

/** Result filter. Denial results (bad_key, locked, rate_limited) are all shown as "denied". */
export const RESULT_OPTIONS: Option[] = [
  { value: '', key: 'audit.filter.all' },
  { value: 'ok', key: 'audit.result.ok' },
  { value: 'failed', key: 'audit.result.failed' },
  { value: 'bad_key', key: 'audit.result.denied' },
  { value: 'locked', key: 'audit.result.denied' },
  { value: 'rate_limited', key: 'audit.result.denied' },
];
