/** Known audit action keys. Each one has an `st.audit.action.<key>` translation; other keys are shown raw. */
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
  'connection.update', 'connection.close_old',
  'assistant.type', 'disk.scan', 'disk.clean', 'docker.check', 'group.reorder', 'join.create', 'join.revoke', 'network.dns', 'server.note', 'server.tags', 'appaccess.update', 'user.access', 'user.password_reset', 'security.fail2ban.ban', 'security.fail2ban.unban', 'security.fail2ban.settings', 'security.key.remove', 'ssh.check', 'ssh.install', 'enroll.join', 'win.users.add', 'win.users.update', 'win.users.passwd', 'win.users.delete', 'win.users.logoff', 'win.network.apply', 'win.network.rollback',
]);

export interface Option {
  value: string;
  key: string;
}

/** Action group filter. The value is sent as the `action` parameter. */
export const ACTION_GROUPS: Option[] = [
  { value: '', key: 'st.audit.filter.all' },
  { value: 'auth.', key: 'st.audit.group.auth' },
  { value: 'user.', key: 'st.audit.group.users' },
  { value: 'snippet.', key: 'st.audit.group.snippets' },
  { value: 'security.', key: 'st.audit.group.security' },
  { value: 'hostuser.', key: 'st.audit.group.hostusers' },
  { value: 'enroll.,server.', key: 'st.audit.group.servers' },
  { value: 'network.', key: 'st.audit.group.network' },
  { value: 'firewall.', key: 'st.audit.group.firewall' },
  { value: 'files.', key: 'st.audit.group.files' },
  { value: 'service.', key: 'st.audit.group.services' },
  { value: 'stream.', key: 'st.audit.group.stream' },
  { value: 'desktop.', key: 'st.audit.group.desktop' },
  { value: 'assistant.', key: 'st.audit.group.assistant' },
  { value: 'update.', key: 'st.audit.group.updates' },
  { value: 'docker.', key: 'st.audit.group.docker' },
  { value: 'process.', key: 'st.audit.group.process' },
  { value: 'metrics.', key: 'st.audit.group.metrics' },
  { value: 'dashboard.', key: 'st.audit.group.dashboard' },
  { value: 'access.', key: 'st.audit.group.access' },
  { value: 'connection.', key: 'st.audit.group.connection' },
];

/** Result filter. Denial results (bad_key, locked, rate_limited) are all shown as "denied". */
export const RESULT_OPTIONS: Option[] = [
  { value: '', key: 'st.audit.filter.all' },
  { value: 'ok', key: 'st.audit.result.ok' },
  { value: 'failed', key: 'st.audit.result.failed' },
  { value: 'bad_key', key: 'st.audit.result.denied' },
  { value: 'locked', key: 'st.audit.result.denied' },
  { value: 'rate_limited', key: 'st.audit.result.denied' },
];
