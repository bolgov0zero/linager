/* Linager interface mockup (variant A "Graphite"). Demo logic only: data is fake, nothing is sent anywhere. */
(() => {
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const ic = (n, s = 16) => `<svg class="ic" width="${s}" height="${s}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICONS[n] || ''}</svg>`;
const esc = s => String(s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
const rng = a => () => { a |= 0; a = a + 0x6D2B79F5 | 0; let t = Math.imul(a ^ a >>> 15, 1 | a); t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t; return ((t ^ t >>> 14) >>> 0) / 4294967296; };
const hash = s => [...s].reduce((h, c) => (h * 31 + c.charCodeAt(0)) | 0, 7);

/* ---------- data ---------- */
const SV = [
  { id: 'web-01', ip: '10.0.1.21', os: 'linux', osn: 'Debian 12', ag: '0.9.1', g: 'Продакшен', cpu: 41, ram: 53, disk: 41, rx: '924 KB/s', tx: '408 KB/s', up: '40д 4ч', st: 'ok', b: [['mute', 'обновлений: 3']] },
  { id: 'web-02', ip: '10.0.1.22', os: 'linux', osn: 'Debian 12', ag: '0.9.1', g: 'Продакшен', cpu: 32, ram: 50, disk: 44, rx: '597 KB/s', tx: '393 KB/s', up: '112д 2ч', st: 'warn', b: [['warn', 'нужна перезагрузка'], ['mute', 'обновлений: 12']] },
  { id: 'proxy-01', ip: '10.0.1.20', os: 'linux', osn: 'Debian 12', ag: '0.9.1', g: 'Продакшен', cpu: 14, ram: 25, disk: 29, rx: '312 KB/s', tx: '290 KB/s', up: '40д 4ч', st: 'ok', b: [['ok', 'без замечаний']] },
  { id: 'cache-01', ip: '10.0.1.13', os: 'linux', osn: 'Ubuntu 24.04', ag: '0.9.1', g: 'Продакшен', cpu: 15, ram: 66, disk: 18, rx: '316 KB/s', tx: '324 KB/s', up: '203д 11ч', st: 'ok', b: [['mute', 'обновлений: 2']] },
  { id: 'db-primary', ip: '10.0.1.14', os: 'linux', osn: 'Debian 12', ag: '0.9.1', g: 'Базы данных', cpu: 51, ram: 79, disk: 63, rx: '624 KB/s', tx: '2.4 MB/s', up: '67д 19ч', st: 'ok', b: [['ok', 'без замечаний']] },
  { id: 'db-replica', ip: '10.0.1.15', os: 'linux', osn: 'Debian 12', ag: '0.9.1', g: 'Базы данных', cpu: 25, ram: 72, disk: 63, rx: '1.8 MB/s', tx: '116 KB/s', up: '67д 19ч', st: 'warn', b: [['crit', 'служб упало: 1'], ['mute', 'обновлений: 5']] },
  { id: 'mail-01', ip: '10.0.4.2', os: 'linux', osn: 'Debian 12', ag: '0.9.1', g: 'Инфраструктура', cpu: 0, ram: 0, disk: 0, rx: '', tx: '', up: 'офлайн 12 мин', st: 'off', b: [['mute', 'нет связи']] },
  { id: 'build-runner', ip: '10.0.1.12', os: 'linux', osn: 'Debian 12', ag: '0.9.1', g: 'Инфраструктура', cpu: 93, ram: 86, disk: 72, rx: '4.4 MB/s', tx: '713 KB/s', up: '9д 1ч', st: 'crit', b: [['mute', 'обновлений: 8']] },
  { id: 'backup-nas', ip: '10.0.1.11', os: 'linux', osn: 'Debian 11', ag: '0.8.4', g: 'Инфраструктура', cpu: 4, ram: 31, disk: 91, rx: '61 KB/s', tx: '2.7 MB/s', up: '330д 7ч', st: 'crit', b: [['warn', 'агент устарел'], ['mute', 'обновлений: 1']] },
  { id: 'monitoring', ip: '10.0.1.18', os: 'linux', osn: 'Debian 12', ag: '0.9.1', g: 'Инфраструктура', cpu: 27, ram: 59, disk: 47, rx: '408 KB/s', tx: '392 KB/s', up: '40д 4ч', st: 'ok', b: [['mute', 'обновлений: 1']] },
  { id: 'office-pc', ip: '10.0.1.19', os: 'windows', osn: 'Windows 11 Pro 24H2', ag: '0.9.1', g: 'Windows', cpu: 22, ram: 62, disk: 55, rx: '463 KB/s', tx: '825 KB/s', up: '12д 6ч', st: 'warn', b: [['warn', 'нужна перезагрузка'], ['mute', 'обновлений: 4']] },
  { id: 'dc-01', ip: '10.0.1.16', os: 'windows', osn: 'Windows Server 2022', ag: '0.9.1', g: 'Windows', cpu: 18, ram: 48, disk: 38, rx: '463 KB/s', tx: '825 KB/s', up: '88д 3ч', st: 'ok', b: [['mute', 'обновлений: 2']] },
];
const byId = id => SV.find(s => s.id === id);
const STL = { ok: 'в норме', warn: 'внимание', crit: 'проблема', off: 'нет связи' };
const lv = (v, w = 75, c = 90) => v >= c ? 'crit' : v >= w ? 'warn' : 'ok';
const GROUPS = [...new Set(SV.map(s => s.g))];
const bad = s => s.st !== 'ok';
const wn = s => s.os === 'windows';

/* ---------- small pieces ---------- */
function series(seed, n = 36, base = 40, amp = 26) { const r = rng(hash(seed)); let v = base; const a = []; for (let i = 0; i < n; i++) { v += (r() - .5) * amp; v = Math.max(6, Math.min(94, v)); a.push(v); } return a; }
function spark(a, w = 120, h = 32, cls = '') {
  const pts = a.map((v, i) => [i * (w / (a.length - 1)), h - (v / 100) * h * .86 - 2]);
  const line = 'M' + pts.map(p => p[0].toFixed(1) + ',' + p[1].toFixed(1)).join(' L');
  return `<svg class="spark ${cls}" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" aria-hidden="true"><path class="sa" d="${line} L${w},${h} L0,${h}Z"/><path class="sl" d="${line}" vector-effect="non-scaling-stroke"/></svg>`;
}
function chart(a, cls = '') {
  const w = 640, h = 150, pts = a.map((v, i) => [i * (w / (a.length - 1)), h - (v / 100) * (h - 14) - 7]);
  const line = 'M' + pts.map(p => p[0].toFixed(1) + ',' + p[1].toFixed(1)).join(' L');
  return `<svg class="chart ${cls}" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" aria-hidden="true">${[.25, .5, .75].map(k => `<line class="gl" x1="0" x2="${w}" y1="${h * k}" y2="${h * k}" vector-effect="non-scaling-stroke"/>`).join('')}<path class="sa" d="${line} L${w},${h} L0,${h}Z"/><path class="sl" d="${line}" vector-effect="non-scaling-stroke"/></svg>`;
}
const badge = ([k, t]) => `<span class="badge b-${k}">${esc(t)}</span>`;
const osTag = s => `<span class="os os-${s.os}">${wn(s) ? 'WIN' : 'LIN'}</span>`;
const dis = s => s.st === 'off' ? 'disabled' : '';

function metrics(s) {
  const rows = [['CPU', s.cpu], ['RAM', s.ram], ['Диск', s.disk]];
  if (s.st === 'off') return `<div class="metrics off">` + rows.map(([l]) => `<div class="m"><span class="ml">${l}</span><span class="m-off">нет данных</span></div>`).join('') + '</div>';
  return `<div class="metrics">` + rows.map(([l, v]) => `<div class="m lv-${lv(v)}"><span class="ml">${l}</span><span class="bar"><i style="width:${v}%"></i></span><span class="mv">${v}%</span></div>`).join('') + '</div>';
}

/* ---------- custom select ---------- */
const SELS = {
  grp: { icon: 'folders', opts: () => [['all', 'Все группы'], ...GROUPS.map(g => [g, g])], get: () => S.group, set: v => { S.group = v; renderDash(); } },
  osf: { opts: () => [['all', 'ОС: все'], ['linux', 'Linux'], ['windows', 'Windows']], get: () => S.osf, set: v => { S.osf = v; renderConsole(); } },
  stf: { opts: () => [['all', 'Статус: все'], ['on', 'в сети'], ['off', 'не в сети']], get: () => S.stf, set: v => { S.stf = v; renderConsole(); } },
};
function selHtml(id, cls = '') {
  const d = SELS[id], cur = d.opts().find(o => o[0] === d.get());
  return `<button type="button" class="csel ${cls}" data-act="selopen" data-sel="${id}" aria-haspopup="listbox">${d.icon ? ic(d.icon, 14) : ''}<span>${cur[1]}</span>${ic('chevron-down', 13)}</button>`;
}
function closePop() { const p = $('.pop'); if (p) p.remove(); $$('.csel[aria-expanded=true]').forEach(b => b.setAttribute('aria-expanded', 'false')); }
function openPop(btn) {
  const had = $('.pop') && $('.pop').dataset.sel === btn.dataset.sel; closePop(); if (had) return;
  const d = SELS[btn.dataset.sel], cur = d.get(), p = document.createElement('div');
  p.className = 'pop'; p.dataset.sel = btn.dataset.sel; p.setAttribute('role', 'listbox');
  p.innerHTML = d.opts().map(([v, t]) => `<button role="option" data-act="selpick" data-sel="${btn.dataset.sel}" data-v="${v}" aria-selected="${v === cur}">${t}${v === cur ? ic('check', 14) : ''}</button>`).join('');
  document.body.appendChild(p);
  const r = btn.getBoundingClientRect(), h = p.offsetHeight;
  p.style.minWidth = r.width + 'px'; p.style.left = Math.min(r.left, innerWidth - p.offsetWidth - 8) + 'px';
  p.style.top = (r.bottom + h + 12 > innerHeight ? r.top - h - 4 : r.bottom + 4) + 'px';
  btn.setAttribute('aria-expanded', 'true');
}

/* ---------- terminal ---------- */
const FILES = [['d', 'backups'], ['d', 'www'], ['d', 'logs'], ['f', 'deploy.sh'], ['f', 'notes.txt'], ['f', 'nginx.conf.bak']];
function termReply(s, raw) {
  const w = wn(s), [c, ...a] = raw.trim().split(/\s+/), cmd = c.toLowerCase(), arg = a.join(' ');
  const G = t => `<span class="c-g">${t}</span>`, B = t => `<span class="c-b">${t}</span>`, Y = t => `<span class="c-y">${t}</span>`, D = t => `<span class="c-d">${t}</span>`, R = t => `<span class="c-r">${t}</span>`;
  if (!c) return '';
  if (cmd === 'help') return w ? 'Демо-команды: hostname, whoami, Get-Date, dir, Get-Service, clear' : 'Демо-команды: help, ls, pwd, whoami, hostname, uptime, date, df -h, free -h, top, systemctl status nginx, echo, clear';
  if (cmd === 'hostname') return s.id;
  if (cmd === 'whoami') return w ? s.id + '\\administrator' : 'admin';
  if (cmd === 'pwd') return '/home/admin';
  if (cmd === 'date' || cmd === 'get-date') return new Date().toLocaleString('ru-RU');
  if (cmd === 'echo') return esc(arg);
  if (cmd === 'ls' || cmd === 'dir') return FILES.map(([t, n]) => t === 'd' ? B(n + '/') : n).join('   ');
  if (cmd === 'uptime') return ` ${new Date().toTimeString().slice(0, 8)} up ${s.up.replace('д', ' days,').replace('ч', 'h')},  1 user,  load average: 0.41, 0.38, 0.35`;
  if (cmd === 'df') return `Filesystem      Size  Used Avail Use% Mounted on\n/dev/vda1        80G   ${Math.round(.8 * s.disk)}G   ${80 - Math.round(.8 * s.disk)}G  ${lv(s.disk) === 'ok' ? s.disk + '%' : Y(s.disk + '%')} /\ntmpfs           3.9G     0  3.9G   0% /dev/shm`;
  if (cmd === 'free') return `               total        used        free      shared  buff/cache\nMem:           7.8Gi       ${(7.8 * s.ram / 100).toFixed(1)}Gi       1.1Gi       112Mi       2.4Gi\nSwap:          2.0Gi          0B       2.0Gi`;
  if (cmd === 'top') return `${D('PID    USER      %CPU  %MEM  COMMAND')}\n 812   www-data   ${(s.cpu * .4).toFixed(1)}   3.1  nginx: worker\n 790   postgres   ${(s.cpu * .3).toFixed(1)}   9.4  postgres: main\n1204   admin       0.7   0.4  sshd: admin@pts/0`;
  if (cmd === 'systemctl' || cmd === 'get-service') return `${G('●')} nginx.service - A high performance web server\n     Loaded: loaded (/lib/systemd/system/nginx.service; enabled)\n     Active: ${G('active (running)')} since Mon 2026-10-12 09:41:07 MSK; 40 days ago\n   Main PID: 790 (nginx)`;
  if (cmd === 'clear' || cmd === 'cls') return null;
  return w ? R(`${esc(c)} : The term '${esc(c)}' is not recognized as the name of a cmdlet.`) : `bash: ${esc(c)}: command not found`;
}
function mkTerm(sid) {
  const s = byId(sid), w = wn(s);
  const ps1 = w ? `<span class="c-y">PS C:\\Users\\admin&gt;</span>` : `<span class="c-g">admin@${s.id}</span>:<span class="c-b">~</span>$`;
  const el = document.createElement('div'); el.className = 'term';
  el.innerHTML = `<div class="t-out"></div><form class="t-in"><span class="ps1">${ps1}</span><input spellcheck="false" autocomplete="off" aria-label="Команда"></form>`;
  const out = $('.t-out', el), inp = $('input', el), hist = []; let hi = 0;
  out.innerHTML = w
    ? `Windows PowerShell\n<span class="c-d">${esc(s.osn)} · агент ${s.ag} · сеанс через Linager</span>\n\n`
    : `Linux ${s.id} 6.1.0-28-amd64 #1 SMP PREEMPT_DYNAMIC Debian 6.1.119-1 x86_64\n<span class="c-d">${esc(s.osn)} · агент ${s.ag} · сеанс через Linager</span>\n\n<span class="c-d">Last login: Sat Nov 22 21:04:11 2026 from 10.0.0.5</span>\n`;
  $('form', el).addEventListener('submit', e => {
    e.preventDefault(); const v = inp.value; inp.value = '';
    if (v.trim()) { hist.push(v); hi = hist.length; }
    const r = termReply(s, v);
    if (r === null) out.innerHTML = ''; else out.innerHTML += `${ps1} ${esc(v)}\n${r ? r + '\n' : ''}`;
    el.scrollTop = el.scrollHeight;
  });
  inp.addEventListener('keydown', e => {
    if (e.key === 'ArrowUp' && hi > 0) { inp.value = hist[--hi]; e.preventDefault(); }
    if (e.key === 'ArrowDown') { inp.value = hi < hist.length - 1 ? hist[++hi] : (hi = hist.length, ''); e.preventDefault(); }
  });
  el.addEventListener('mousedown', e => { if (!window.getSelection().toString() && e.target.tagName !== 'INPUT') setTimeout(() => inp.focus(), 0); });
  el.focusIn = () => inp.focus();
  return el;
}
function desktop(s) {
  return `<div class="rd"><div class="rd-win"><div class="rd-tb"><i></i><i></i><i></i><span>${s.id} — Диспетчер задач</span></div><div class="rd-body"><div class="rd-l w1"></div><div class="rd-l w2"></div><div class="rd-l w3"></div><div class="rd-l w4"></div><div class="rd-l w5"></div><div class="rd-hl"></div></div></div><div class="rd-task"><i class="st"></i><i></i><i></i><i></i><span class="grow"></span><b>12:00</b></div></div>`;
}
const rdInfo = `<div class="rd-info"><span>1920×1080</span><span>цвета: 24 бита</span><span>кадров: 13 из 15</span><span>трафик: 0,3 Мбит/с <i class="dim">(лимит 4 Мбит/с)</i></span></div>`;
const rdTools = id => `<button class="btn sm" data-act="soon" data-m="Список сеансов Windows">Сеанс${ic('chevron-down', 13)}</button><button class="btn sm" data-act="soon" data-m="Ctrl+Alt+Del отправлен">Ctrl+Alt+Del</button><button class="btn sm" data-act="files" data-id="${id}">${ic('folder', 14)}Файлы</button><button class="btn sm" data-act="proc" data-id="${id}">${ic('activity', 14)}Диспетчер</button><button class="btn sm" data-act="soon" data-m="Качество и частота кадров: отдельное окно">Ещё${ic('chevron-down', 13)}</button>`;

/* ---------- state ---------- */
const S = { view: 'dash', filter: 'all', group: 'all', q: '', layout: 'grid', mode: 'term', tabs: [], active: null, wins: [], z: 10, collapsed: {}, osf: 'all', stf: 'all', cq: '', sid: null, stab: 'overview', sterms: {}, settingsTab: 'profile', spc: false, sgc: {}, notes: { 'web-02': 'Доступы: ключ у ivan.petrov.\nПерезагрузить после 1 декабря, когда закончится окно релиза.\nВнешний IP за балансировщиком: 203.0.113.22' } };
let tabSeq = 0, winSeq = 0;
const modeIcon = { term: 'square-terminal', rdp: 'monitor', assist: 'app-window' };
const modeName = { term: 'Терминал', rdp: 'RDP', assist: 'Помощник' };
const modeOk = (s, m) => s.st !== 'off' && (m === 'term' || wn(s));

/* ---------- shell ---------- */
function shell() {
  const nav = [['dash', 'layout-grid', 'Дашборд'], ['console', 'square-terminal', 'Консоль']];
  $('#app').innerHTML = `
  <div class="app">
    <header class="topbar">
      <div class="brand"><div class="logo">${ic('terminal', 15)}</div><b>Linager</b></div>
      <nav class="seg nav-tabs" role="tablist">${nav.map(([k, i, t]) => `<button data-act="view" data-v="${k}" role="tab">${ic(i, 15)}${t}</button>`).join('')}</nav>
      <span class="grow"></span>
      <div class="top-actions">
        <button class="btn ghost" data-act="soon" data-m="Поиск по всем серверам: в макете не реализован">${ic('search', 15)}<span>Поиск</span><kbd>⌘K</kbd></button>
        <button class="btn ghost" data-act="settings" data-s="updates">${ic('download', 15)}<span>Обновления</span><em class="cnt">3</em></button>
        <button class="btn primary" data-act="soon" data-m="Добавление сервера: в макете не реализовано">${ic('plus', 15)}<span>Сервер</span></button>
        <button class="btn icon ghost" data-act="theme" title="Тема">${ic('sun', 16)}</button>
        <button class="btn icon ghost" data-act="settings" data-s="profile" title="Настройки">${ic('settings', 16)}</button>
      </div>
    </header>
    <main id="view" class="view"></main>
    <div id="wins"></div><div id="dock" class="dock"></div><div id="toast" class="toast"></div>
  </div>`;
}
function toast(m) { const t = $('#toast'); t.textContent = m; t.classList.add('on'); clearTimeout(toast.t); toast.t = setTimeout(() => t.classList.remove('on'), 2400); }
function setView(v) {
  S.view = v;
  $$('[data-act=view]').forEach(b => { const on = b.dataset.v === v || (v === 'server' && b.dataset.v === 'dash'); b.classList.toggle('on', on); b.setAttribute('aria-selected', on); });
  $('#view').className = 'view v-' + v; $('#view').scrollTop = 0;
  ({ dash: renderDash, console: renderConsole, server: renderServer })[v]();
}

/* ---------- dashboard ---------- */
function filtered() {
  return SV.filter(s => (S.filter === 'all' || (S.filter === 'bad' && bad(s) && s.st !== 'off') || (S.filter === 'off' && s.st === 'off')) && (S.group === 'all' || s.g === S.group) && (!S.q || (s.id + s.ip).includes(S.q.toLowerCase())));
}
function tiles() {
  const on = SV.filter(s => s.st !== 'off'), cpu = Math.round(on.reduce((a, s) => a + s.cpu, 0) / on.length);
  const T = [
    ['Онлайн', `${on.length}<small>/${SV.length}</small>`, 'нет связи: 1', 'crit', 'server', series('on', 30, 70, 8)],
    ['Внимание', SV.filter(s => s.st === 'warn' || s.st === 'crit').length, 'критичных: 2', 'warn', 'triangle-alert', series('at', 30, 45, 30)],
    ['Обновления', 38, 'ждут перезагрузки: 2', '', 'download', series('up', 30, 55, 14)],
    ['CPU средн.', `${cpu}<small>%</small>`, 'по серверам online', '', 'activity', series('cpu', 30, 40, 28)],
  ];
  return `<section class="tiles">` + T.map(([k, v, s, tone, i, d]) => `<div class="tile ${tone ? 'tone-' + tone : ''}"><div class="tile-h"><span>${k}</span>${ic(i, 14)}</div><div class="tile-v">${v}</div><div class="tile-s">${s}</div>${spark(d, 120, 36)}</div>`).join('') + `</section>`;
}
const winBtns = s => `<button class="btn icon sm" data-act="term" data-id="${s.id}" title="Терминал" ${dis(s)}>${ic('square-terminal', 15)}</button>` +
  (wn(s) ? `<button class="btn icon sm" data-act="rdp" data-id="${s.id}" title="RDP" ${dis(s)}>${ic('monitor', 15)}</button><button class="btn icon sm" data-act="assist" data-id="${s.id}" title="Помощник" ${dis(s)}>${ic('app-window', 15)}</button>` : '') +
  `<button class="btn icon sm" data-act="files" data-id="${s.id}" title="Файлы" ${dis(s)}>${ic('folder', 15)}</button>`;
function card(s) {
  return `<article class="card st-${s.st}" data-act="server" data-id="${s.id}">
    <header class="c-head"><span class="dot ${s.st}" title="${STL[s.st]}"></span><h3 data-act="server" data-id="${s.id}">${s.id}</h3>${osTag(s)}</header>
    <p class="c-sub"><span class="mono">${s.ip}</span><i>·</i>${esc(s.osn)}<i>·</i>агент ${s.ag}</p>
    <div class="badges">${s.b.map(badge).join('')}</div>
    ${metrics(s)}
    <div class="net">${s.st === 'off' ? '<span class="net-off"></span>' : spark(series(s.id + 'n', 40, 35, 40), 160, 26, 'net-spark')}<div class="net-v"><span>${ic('arrow-down', 11)}${s.rx || '—'}</span><span>${ic('arrow-up', 11)}${s.tx || '—'}</span></div></div>
    <footer class="c-foot"><span class="up">${s.st === 'off' ? s.up : 'uptime ' + s.up}</span><div class="acts">${winBtns(s)}<button class="btn icon sm" data-act="server" data-id="${s.id}" title="Страница сервера">${ic('external-link', 15)}</button></div></footer></article>`;
}
function mini(s) {
  const cols = [['CPU', s.cpu], ['RAM', s.ram], ['Диск', s.disk]];
  const note = s.b.find(b => b[0] === 'crit' || b[0] === 'warn') || (s.st === 'off' ? s.b[0] : null);
  return `<article class="card mini st-${s.st}" data-act="server" data-id="${s.id}">
    <header class="c-head"><span class="dot ${s.st}" title="${STL[s.st]}"></span><h3 data-act="server" data-id="${s.id}">${s.id}</h3>${osTag(s)}</header>
    <div class="mini-m">${cols.map(([l, v]) => `<div class="lv-${lv(v)}"><small>${l}</small><b>${s.st === 'off' ? '—' : v}</b><span class="bar"><i style="width:${s.st === 'off' ? 0 : v}%"></i></span></div>`).join('')}</div>
    <footer class="mini-f"><span class="mono ${note ? 'warnt' : 'dim'}" title="${note ? esc(note[1]) : ''}">${note ? esc(note[1]) : s.ip}</span><div class="acts"><button class="btn icon xs" data-act="term" data-id="${s.id}" title="Терминал" ${dis(s)}>${ic('square-terminal', 14)}</button>${wn(s) ? `<button class="btn icon xs" data-act="assist" data-id="${s.id}" title="Помощник" ${dis(s)}>${ic('app-window', 14)}</button>` : ''}<button class="btn icon xs" data-act="server" data-id="${s.id}" title="Страница сервера">${ic('external-link', 14)}</button></div></footer></article>`;
}
function row(s) {
  const m = v => `<span class="m lv-${lv(v)}"><span class="bar"><i style="width:${v}%"></i></span><b>${s.st === 'off' ? '—' : v + '%'}</b></span>`;
  return `<tr class="st-${s.st}" data-act="server" data-id="${s.id}"><td><span class="dot ${s.st}"></span><b data-act="server" data-id="${s.id}" class="lnk">${s.id}</b> ${osTag(s)}</td><td class="mono">${s.ip}</td><td>${esc(s.osn)}</td><td>${m(s.cpu)}</td><td>${m(s.ram)}</td><td>${m(s.disk)}</td><td>${s.b.map(badge).join('')}</td><td class="mono dim">${s.up}</td><td class="r">${winBtns(s)}</td></tr>`;
}
function renderDash() {
  $('#view').innerHTML = `${tiles()}
    <div class="toolbar"><div id="chips" class="chips"></div><span class="grow"></span>${selHtml('grp')}
      <label class="field">${ic('search', 14)}<input id="q" placeholder="Поиск по имени или IP" value="${esc(S.q)}"></label>
      <div class="seg" id="lay"><button data-act="layout" data-l="grid" title="Карточки">${ic('layout-grid', 14)}</button><button data-act="layout" data-l="mini" title="Компактные карточки">${ic('layers', 14)}</button><button data-act="layout" data-l="list" title="Строки">${ic('rows-3', 14)}</button></div></div>
    <div id="grid"></div>`;
  $('#q').oninput = e => { S.q = e.target.value; updGrid(); };
  updGrid();
}
function updGrid() {
  const list = filtered();
  const cnt = { all: SV.length, bad: SV.filter(s => bad(s) && s.st !== 'off').length, off: SV.filter(s => s.st === 'off').length };
  $('#chips').innerHTML = [['all', 'Все'], ['bad', 'Проблемные'], ['off', 'Offline']].map(([k, t]) => `<button class="chip" data-act="filter" data-f="${k}" aria-pressed="${S.filter === k}">${t}<em>${cnt[k]}</em></button>`).join('');
  $$('#lay button').forEach(b => b.setAttribute('aria-pressed', b.dataset.l === S.layout));
  const g = $('#grid'); g.className = S.layout === 'list' ? 'tbl-wrap' : S.layout === 'mini' ? 'grid mini-grid' : 'grid';
  g.innerHTML = !list.length ? `<div class="empty">${ic('search', 26)}<p>Ничего не найдено</p></div>` : S.layout === 'list'
    ? `<table class="tbl"><thead><tr><th>Сервер</th><th>IP</th><th>Система</th><th>CPU</th><th>RAM</th><th>Диск</th><th>Состояние</th><th>Uptime</th><th></th></tr></thead><tbody>${list.map(row).join('')}</tbody></table>`
    : list.map(S.layout === 'mini' ? mini : card).join('');
}

/* ---------- console ---------- */
const curTab = () => S.tabs.find(t => t.id === S.active);
function renderConsole() {
  const osOk = s => S.osf === 'all' || s.os === S.osf, stOk = s => S.stf === 'all' || (S.stf === 'on' ? s.st !== 'off' : s.st === 'off');
  const cnt = m => SV.filter(s => modeOk(s, m) && osOk(s) && stOk(s)).length, q = S.cq.toLowerCase();
  $('#view').innerHTML = `<div class="console">
   <aside class="c-side">
     <div class="mode-sw">${['term', 'rdp', 'assist'].map(m => `<button data-act="mode" data-m="${m}" aria-pressed="${S.mode === m}" title="${modeName[m]}" aria-label="${modeName[m]}">${ic(modeIcon[m], 16)}<em>${cnt(m)}</em></button>`).join('')}</div>
     <label class="field c-find">${ic('search', 14)}<input id="cq" placeholder="Фильтр серверов" value="${esc(S.cq)}"></label>
     <div class="c-list">${GROUPS.map(g => {
       const items = SV.filter(s => s.g === g && modeOk({ ...s, st: 'ok' }, S.mode) && osOk(s) && stOk(s) && (!q || s.id.includes(q)));
       if (!items.length) return '';
       return `<div class="c-grp"><button class="c-grp-h" data-act="grp" data-g="${g}">${ic(S.collapsed[g] ? 'chevron-right' : 'chevron-down', 13)}${g}<em>${items.length}</em></button>${S.collapsed[g] ? '' : items.map(s => `<button class="c-item ${curTab() && curTab().sid === s.id && curTab().mode === S.mode ? 'on' : ''}" data-act="csrv" data-id="${s.id}" ${dis(s)}><span class="dot ${s.st}"></span><span class="nm">${s.id}</span>${osTag(s)}</button>`).join('')}</div>`;
     }).join('')}</div>
     <div class="c-filters"><span>${ic('list-filter', 13)} Фильтры</span>${selHtml('osf', 'sm')}${selHtml('stf', 'sm')}</div>
   </aside>
   <section class="c-main"><div class="c-tabs" id="ctabs"></div><div class="c-pane" id="cpane"></div></section></div>`;
  $('#cq').oninput = e => { S.cq = e.target.value; const p = e.target.selectionStart; renderConsole(); const n = $('#cq'); n.focus(); n.setSelectionRange(p, p); };
  renderTabs();
}
function renderTabs() {
  $('#ctabs').innerHTML = S.tabs.map(t => { const s = byId(t.sid); return `<div class="c-tab ${t.id === S.active ? 'on' : ''}" data-act="tab" data-t="${t.id}"><span class="dot ${s.st}"></span>${ic(modeIcon[t.mode], 13)}<span>${t.sid}</span><button data-act="tabx" data-t="${t.id}" title="Закрыть">${ic('x', 12)}</button></div>`; }).join('');
  const pane = $('#cpane'); pane.innerHTML = '';
  const t = curTab();
  if (!t) { pane.innerHTML = `<div class="empty big">${ic('square-terminal', 32)}<h3>Выберите сервер слева</h3><p>Терминал, RDP или Помощник откроются во вкладке. Несколько сеансов живут рядом и не мешают друг другу.</p></div>`; return; }
  const s = byId(t.sid), bar = document.createElement('div'); bar.className = 'c-bar';
  bar.innerHTML = `<div class="c-bar-t"><span class="dot ${s.st}"></span>${modeName[t.mode]}<i>·</i><b>${s.id}</b><i>·</i><span class="dim">подключено</span></div><span class="grow"></span>` +
    (t.mode === 'term'
      ? `<button class="btn sm" data-act="soon" data-m="Сниппеты настраиваются в «Настройках»">${ic('file-code', 14)}Сниппеты${ic('chevron-down', 13)}</button><button class="btn sm" data-act="files" data-id="${s.id}">${ic('folder', 14)}Файлы</button><button class="btn sm" data-act="proc" data-id="${s.id}">${ic('activity', 14)}Диспетчер</button>`
      : rdTools(s.id)) +
    `<button class="btn sm icon" data-act="win" data-k="${t.mode}" data-id="${s.id}" title="В плавающее окно">${ic('maximize-2', 14)}</button><button class="btn sm icon" data-act="server" data-id="${s.id}" title="Страница сервера">${ic('external-link', 14)}</button>`;
  pane.appendChild(bar);
  if (t.mode === 'term') { if (!t.el) t.el = mkTerm(t.sid); pane.appendChild(t.el); setTimeout(() => t.el.focusIn(), 0); }
  else { if (!t.el) { t.el = document.createElement('div'); t.el.className = 'rd-wrap'; t.el.innerHTML = desktop(s); } pane.appendChild(t.el); pane.insertAdjacentHTML('beforeend', rdInfo); }
}
function openTab(sid, mode) {
  let t = S.tabs.find(x => x.sid === sid && x.mode === mode);
  if (!t) { t = { id: ++tabSeq, sid, mode }; S.tabs.push(t); }
  S.active = t.id; S.mode = mode;
}

/* ---------- file manager (tree + table): server page tab and floating window ---------- */
const FSD = {
  linux: { root: '/', dirs: { '/': ['bin', 'boot', 'dev', 'etc', 'home', 'lib', 'opt', 'root', 'srv', 'usr', 'var'], '/etc': ['apt', 'cron.d', 'docker', 'fail2ban', 'nginx', 'ssh', 'systemd'], '/etc/nginx': ['conf.d', 'sites-available', 'sites-enabled', 'snippets'], '/etc/apt': ['sources.list.d'], '/home': ['admin'], '/home/admin': ['backups', 'www', '.config'], '/var': ['lib', 'log', 'www'], '/var/log': ['apt', 'nginx'], '/var/www': ['html'], '/usr': ['bin', 'lib', 'local', 'share'] },
    files: { '/etc': [['fstab', '-rw-r--r--', 'root', '742 Б', '12 окт 09:40'], ['hostname', '-rw-r--r--', 'root', '7 Б', '12 окт 09:40'], ['hosts', '-rw-r--r--', 'root', '221 Б', '12 окт 09:40'], ['passwd', '-rw-r--r--', 'root', '1.9 КБ', '02 ноя 16:05']],
      '/etc/nginx': [['dhparam.pem', '-rw-------', 'root', '424 Б', '17 ноя 10:55'], ['fastcgi_params', '-rw-r--r--', 'root', '1007 Б', '17 ноя 10:55'], ['mime.types', '-rw-r--r--', 'root', '5.2 КБ', '17 ноя 10:55'], ['nginx.conf', '-rw-r--r--', 'root', '1.5 КБ', '17 ноя 10:55'], ['proxy_params', '-rw-r--r--', 'root', '441 Б', '17 ноя 10:55'], ['uwsgi_params', '-rw-r--r--', 'root', '664 Б', '17 ноя 10:55']],
      '/home/admin': [['.bashrc', '-rw-r--r--', 'admin', '3.7 КБ', '12 окт 09:41'], ['deploy.sh', '-rwxr-xr-x', 'admin', '1.2 КБ', '18 ноя 10:31'], ['notes.txt', '-rw-r--r--', 'admin', '812 Б', '02 ноя 16:05'], ['site.tar.gz', '-rw-r--r--', 'admin', '48 МБ', '21 ноя 03:02']],
      '/var/log': [['auth.log', '-rw-r-----', 'syslog', '212 КБ', '23 ноя 12:01'], ['dpkg.log', '-rw-r--r--', 'root', '96 КБ', '22 ноя 09:12'], ['syslog', '-rw-r-----', 'syslog', '1.4 МБ', '23 ноя 12:01']] } },
  windows: { root: 'C:', dirs: { '/': ['inetpub', 'Program Files', 'Temp', 'Users', 'Windows'], '/Users': ['Administrator', 'Public'], '/Users/Administrator': ['Desktop', 'Documents', 'Downloads'], '/Windows': ['Logs', 'System32'] },
    files: { '/Temp': [['setup.log', '', 'SYSTEM', '48 КБ', '22 ноя 09:12']], '/Users/Administrator': [['notes.txt', '', 'Administrator', '812 Б', '02 ноя 16:05']], '/inetpub': [['web.config', '', 'SYSTEM', '2.1 КБ', '12 окт 09:40']] } },
};
const FM = {}; let fmSeq = 0;
const jn = (p, k) => p === '/' ? '/' + k : p + '/' + k;
const fsOf = sid => FSD[wn(byId(sid)) ? 'windows' : 'linux'];
function fmOpenTo(st, p) { const parts = p.split('/').filter(Boolean); let c = '/'; st.open.add('/'); parts.forEach(x => { c = jn(c, x); st.open.add(c); }); }
function fmHtml(sid, cls = '') {
  const id = 'fm' + (++fmSeq), st = FM[id] = { sid, path: wn(byId(sid)) ? '/Users/Administrator' : '/etc/nginx', open: new Set() };
  fmOpenTo(st, st.path);
  return `<div class="fmv ${cls}" data-fm="${id}">${fmInner(id)}</div>`;
}
function fmNode(st, fs, p, name, depth) {
  const kids = fs.dirs[p] || [], open = st.open.has(p);
  return `<div class="tn"><div class="tr ${st.path === p ? 'on' : ''}" style="padding-left:${4 + depth * 14}px"><button class="tg ${kids.length ? '' : 'nil'}" data-act="fm-tog" data-p="${p}" aria-label="Раскрыть">${ic(open ? 'chevron-down' : 'chevron-right', 13)}</button><button class="tl" data-act="fm-go" data-p="${p}">${ic('folder', 14)}<span>${name}</span></button></div>${open ? kids.map(k => fmNode(st, fs, jn(p, k), k, depth + 1)).join('') : ''}</div>`;
}
function fmInner(id) {
  const st = FM[id], fs = fsOf(st.sid), parts = st.path.split('/').filter(Boolean);
  const dirs = (fs.dirs[st.path] || []).map(d => [d, 'd']), files = (fs.files[st.path] || []).map(f => [f[0], 'f', ...f.slice(1)]);
  const rows = [...dirs.map(([n]) => `<tr data-act="fm-go" data-p="${jn(st.path, n)}" class="fr"><td>${ic('folder', 14)}<b>${n}/</b></td><td class="mono dim">${wn(byId(st.sid)) ? '' : 'drwxr-xr-x'}</td><td class="dim">${wn(byId(st.sid)) ? 'SYSTEM' : 'root'}</td><td class="mono dim r">—</td><td class="mono dim">20 ноя 10:55</td><td></td></tr>`),
    ...files.map(([n, , pr, ow, sz, dt]) => `<tr data-act="fm-edit" data-p="${jn(st.path, n)}" class="fr"><td>${ic('file-text', 14)}${n}</td><td class="mono dim">${pr}</td><td class="dim">${ow}</td><td class="mono r">${sz}</td><td class="mono dim">${dt}</td><td class="r"><button class="btn xs" data-act="fm-edit" data-p="${jn(st.path, n)}" title="Редактировать">${ic('pencil', 13)}</button></td></tr>`)];
  return `<div class="fm-bar"><div class="crumbs"><button data-act="fm-go" data-p="/">${fs.root}</button>${parts.map((x, i) => `<i>/</i><button data-act="fm-go" data-p="/${parts.slice(0, i + 1).join('/')}">${x}</button>`).join('')}</div><span class="grow"></span>
    <button class="btn sm" data-act="fm-go" data-p="${'/' + parts.slice(0, -1).join('/')}" ${parts.length ? '' : 'disabled'}>${ic('folder-up', 14)}Вверх</button><button class="btn sm" data-act="soon" data-m="Новая папка (демо)">${ic('folder-plus', 14)}Папка</button><button class="btn sm" data-act="soon" data-m="Новый файл (демо)">${ic('file-plus', 14)}Файл</button><button class="btn sm" data-act="soon" data-m="Загрузка файлов в макете не реализована">${ic('upload', 14)}Загрузить</button><button class="btn sm icon" data-act="fm-go" data-p="${st.path}" title="Обновить">${ic('refresh-cw', 14)}</button></div>
  <div class="fm-split"><div class="fm-tree"><div class="tree">${fmNode(st, fs, '/', fs.root, 0)}</div></div>
  <div class="fm-list"><table class="tbl compact"><thead><tr><th>Имя</th><th>Права</th><th>Владелец</th><th class="r">Размер</th><th>Изменён</th><th></th></tr></thead><tbody>${rows.join('') || `<tr><td colspan="6" class="dim">Папка пуста</td></tr>`}</tbody></table></div></div>`;
}
function fmUpdate(t, mut) {
  const el = t.closest('[data-fm]'), id = el.dataset.fm, tw = $('.fm-tree', el) && $('.fm-tree', el).style.width;
  mut(FM[id]); el.innerHTML = fmInner(id); if (tw) $('.fm-tree', el).style.width = tw;
}
const filesHtml = sid => fmHtml(sid);

/* ---------- file editor window ---------- */
const TXT = {
  'nginx.conf': `user www-data;\nworker_processes auto;\npid /run/nginx.pid;\n\nevents {\n    worker_connections 1024;\n}\n\nhttp {\n    sendfile on;\n    keepalive_timeout 65;\n    include /etc/nginx/mime.types;\n    include /etc/nginx/conf.d/*.conf;\n    include /etc/nginx/sites-enabled/*;\n\n    access_log /var/log/nginx/access.log;\n    error_log  /var/log/nginx/error.log warn;\n}\n`,
  'deploy.sh': `#!/usr/bin/env bash\nset -euo pipefail\n\nAPP=/var/www/html\ncd "$APP"\ngit pull --ff-only\nnpm ci --omit=dev\nsudo systemctl reload nginx\necho "deployed $(date -Is)"\n`,
  'notes.txt': `Заметки по серверу\n\n- ротация логов настроена в /etc/logrotate.d\n- бэкап базы каждую ночь в 03:00\n- план: обновить ядро после 1 декабря\n`,
  fstab: `# <file system> <mount point> <type> <options> <dump> <pass>\nUUID=3f0c-9a1e  /      ext4  errors=remount-ro  0  1\nUUID=7b21-44d0  /var   ext4  defaults           0  2\n`,
};
const fileText = name => TXT[name] || `# ${name}\n# демонстрационное содержимое файла\n`;
function mkEditor(w) {
  const path = w.opt.path, name = path.split('/').pop(), orig0 = fileText(name);
  const el = document.createElement('div'); el.className = 'edw'; let orig = orig0;
  el.innerHTML = `<div class="ed-bar"><span class="mono ed-path">${esc(path)}</span><span class="ed-mod" hidden>${ic('pencil', 12)}изменён</span><span class="grow"></span><button class="btn sm" data-act="wclose">Закрыть</button><button class="btn sm primary" data-act="ed-save" disabled>${ic('save', 14)}Сохранить</button></div>
  <div class="ed"><div class="ed-g mono"></div><textarea spellcheck="false" wrap="off" aria-label="Содержимое файла"></textarea></div>
  <div class="ed-s mono"><span>UTF-8</span><span>LF</span><span>${/\.(sh)$/.test(name) ? 'Shell' : /conf|params/.test(name) ? 'Конфигурация' : 'Текст'}</span><span class="grow"></span><span class="ed-pos">стр 1, стб 1</span><span class="ed-n"></span></div>`;
  const ta = $('textarea', el), g = $('.ed-g', el); ta.value = orig0;
  const upd = () => {
    const n = ta.value.split('\n').length; g.textContent = Array.from({ length: n }, (_, i) => i + 1).join('\n'); $('.ed-n', el).textContent = n + ' стр.';
    const mod = ta.value !== orig; $('.ed-mod', el).hidden = !mod; $('[data-act=ed-save]', el).disabled = !mod;
    const b = ta.value.slice(0, ta.selectionStart).split('\n'); $('.ed-pos', el).textContent = `стр ${b.length}, стб ${b[b.length - 1].length + 1}`;
  };
  ta.addEventListener('input', upd); ta.addEventListener('keyup', upd); ta.addEventListener('click', upd);
  ta.addEventListener('scroll', () => { g.scrollTop = ta.scrollTop; });
  ta.addEventListener('keydown', e => { if ((e.metaKey || e.ctrlKey) && e.key === 's') { e.preventDefault(); el.save(); } if (e.key === 'Tab') { e.preventDefault(); const s = ta.selectionStart; ta.setRangeText('    ', s, ta.selectionEnd, 'end'); upd(); } });
  el.save = () => { orig = ta.value; upd(); toast('Сохранено (демо): ' + path); };
  el.focusIn = () => ta.focus(); upd(); return el;
}

/* ---------- server note window ---------- */
function mkNote(w) {
  const el = document.createElement('div'); el.className = 'edw note-w';
  el.innerHTML = `<textarea spellcheck="false" placeholder="Заметка о сервере: доступы, особенности, ссылки…" aria-label="Заметка"></textarea><div class="ed-s mono"><span class="nt-st">${S.notes[w.sid] ? 'сохранено' : 'пусто'}</span><span class="grow"></span><span>сохраняется автоматически</span></div>`;
  const ta = $('textarea', el); ta.value = S.notes[w.sid] || '';
  ta.addEventListener('input', () => { S.notes[w.sid] = ta.value; $('.nt-st', el).textContent = ta.value ? 'сохранено' : 'пусто'; const b = $(`[data-act=note][data-id="${w.sid}"]`); if (b) { $('.nd', b) && $('.nd', b).remove(); if (ta.value) b.insertAdjacentHTML('beforeend', '<i class="nd"></i>'); } });
  el.focusIn = () => ta.focus(); return el;
}

/* ---------- server page ---------- */
const updN = s => { const b = s.b.find(x => /обновлений/.test(x[1])); return b ? b[1].split(': ')[1] : null; };
function navFor(s) {
  const W = wn(s);
  return [
    ['Общее', [['overview', 'layout-grid', 'Обзор'], ['metrics', 'activity', 'Метрики'], ['terminal', 'square-terminal', W ? 'PowerShell' : 'Терминал', 'term'], ...(W ? [['rdp', 'monitor', 'RDP', 'rdp'], ['assist', 'app-window', 'Помощник', 'assist']] : [])]],
    ['Система', [['services', 'list-checks', 'Службы'], ['processes', 'cpu', 'Процессы'], ['logs', 'scroll-text', 'Логи'], ['updates', 'download', 'Обновления', null, updN(s)]]],
    ['Файловая система', [['files', 'folder', 'Файлы'], ['disk', 'hard-drive', 'Диск']]],
    ['Сеть и защита', W ? [['network', 'network', 'Сеть'], ['manage', 'sliders-horizontal', 'Управление'], ['users', 'users', 'Пользователи']] : [['network', 'network', 'Сеть'], ['firewall', 'shield-check', 'Файрвол'], ['security', 'lock', 'Безопасность'], ['users', 'users', 'Пользователи']]],
    ...(W ? [] : [['Приложения', [['docker', 'container', 'Docker']]]]),
  ];
}
function renderServer() {
  const s = byId(S.sid), nav = navFor(s), flat = nav.flatMap(g => g[1]);
  if (!flat.find(i => i[0] === S.stab)) S.stab = 'overview';
  const cur = flat.find(i => i[0] === S.stab), grp = nav.find(g => g[1].includes(cur))[0];
  const reboot = s.b.some(b => /перезагрузка/.test(b[1]));
  $('#view').innerHTML = `<div class="sp ${S.spc ? 'collapsed' : ''}">
    <aside class="sp-nav">
      <div class="sp-id"><button class="btn ghost sm back" data-act="view" data-v="dash" title="К списку серверов">${ic('arrow-left', 14)}<span>Серверы</span></button>
        <div class="sp-name"><span class="dot ${s.st}" title="${STL[s.st]}"></span><b>${s.id}</b>${osTag(s)}</div><div class="sp-ip mono">${s.ip}</div></div>
      <div class="sp-groups">${nav.map(([g, items]) => `<div class="sg"><button class="sg-h" data-act="sgrp" data-g="${g}"><span>${g}</span>${ic(S.sgc[g] ? 'chevron-right' : 'chevron-down', 13)}</button>${S.sgc[g] ? '' : items.map(([k, i, t, w, n]) => `<div class="si-row"><button class="si" data-act="stab" data-t="${k}" aria-current="${S.stab === k}" title="${t}">${ic(i, 15)}<span>${t}</span>${n ? `<em class="cnt">${n}</em>` : ''}</button>${w ? `<button class="si-w" data-act="${w}" data-id="${s.id}" title="Открыть в окне" ${dis(s)}>${ic('app-window', 13)}</button>` : ''}</div>`).join('')}</div>`).join('')}</div>
      <div class="sp-foot"><span class="mono">agent ${s.ag}</span><button class="btn xs" data-act="spc" title="${S.spc ? 'Развернуть панель' : 'Свернуть панель'}">${ic(S.spc ? 'panel-left-open' : 'panel-left-close', 15)}</button></div>
    </aside>
    <section class="sp-content">
      <div class="sp-ph"><div><small>${grp}</small><h1>${cur[2]}</h1></div><span class="grow"></span>
        <button class="btn sm" data-act="note" data-id="${s.id}">${ic('sticky-note', 14)}Заметка${S.notes[s.id] ? '<i class="nd"></i>' : ''}</button>
        <button class="btn sm" data-act="confirm" data-t="Перезагрузка" data-m="Сервер ${s.id} будет перезагружен, сеансы и соединения оборвутся. Продолжить?" data-b="Перезагрузить" ${dis(s)}>${ic('power', 14)}Перезагрузить</button>
        <button class="btn sm danger" data-act="confirm" data-t="Удаление сервера" data-m="Сервер ${s.id} будет удалён из Linager. Агент на самом сервере останется. Это действие нельзя отменить." data-b="Удалить" title="Удалить сервер">${ic('trash-2', 14)}</button></div>
      ${reboot ? `<div class="banner">${ic('triangle-alert', 14)}Нужна перезагрузка сервера</div>` : ''}
      <div class="sp-body" id="spb"></div></section></div>`;
  const b = $('#spb'), r = SP[S.stab](s);
  if (typeof r === 'string') b.innerHTML = r; else b.appendChild(r);
  if (S.stab === 'terminal') { const e = $('.term', b); e && e.focusIn && e.focusIn(); }
}
const dl = (k, v) => `<div class="dl"><span>${k}</span><b>${v}</b></div>`;
const T = (h, r, c = '') => `<div class="panel flush"><table class="tbl ${c}"><thead><tr>${h.map(x => `<th>${x}</th>`).join('')}</tr></thead><tbody>${r.map(x => `<tr>${x.map(c => `<td>${c}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`;
const bar = (v, off) => `<span class="m lv-${lv(v)}"><span class="bar"><i style="width:${v}%"></i></span><b>${off ? '—' : v + '%'}</b></span>`;
const svcRows = s => [['nginx', 'HTTP-сервер', 1], ['postgresql', 'База данных PostgreSQL', s.id === 'db-replica' ? 0 : 1], ['redis-server', 'Кэш Redis', 1], ['ssh', 'Сервер OpenSSH', 1], ['cron', 'Планировщик задач', 1], ['linager-agent', 'Агент Linager', 1]];
const tools = (...h) => `<div class="sp-tools">${h.join('')}</div>`;
const sp = (a, c) => `<span class="${c || 'dim'}">${a}</span>`;
const SP = {
  overview: s => {
    const off = s.st === 'off', W = wn(s), g = 8;
    const mc = (t, v, sub, d, cl = '') => `<div class="mc ${cl}"><div class="mc-h">${t}</div><div class="mc-v">${v}</div><div class="mc-s">${sub}</div>${spark(d, 120, 34)}</div>`;
    const disks = W ? [['C:', s.disk, 100], ['D:', 24, 500]] : [['/', s.disk, 100], ['/data', 24, 500]];
    return `<div class="mcs">${mc('CPU', off ? '—' : s.cpu + '<small>%</small>', 'load average: 1.44 · 1.20 · 1.03', series(s.id + 'c', 30, s.cpu, 22), 'lv-' + lv(s.cpu))}
      ${mc('RAM', off ? '—' : s.ram + '<small>%</small><small>· ' + (g * s.ram / 100).toFixed(1) + ' GB</small>', `всего ${g}.0 GB · swap 0 B`, series(s.id + 'r', 30, s.ram, 10), 'lv-' + lv(s.ram))}
      ${mc('Сеть · приём', off ? '—' : s.rx.replace(' ', '<small> ') + '</small>', off ? '' : '↑ отдача ' + s.tx, series(s.id + 'n', 30, 35, 40))}
      <div class="mc disks"><div class="mc-h">Диски</div>${disks.map(([a, v, t]) => `<div class="dk"><div><span class="mono">${a}</span><b class="mono">${off ? '—' : v + '%'}</b></div><span class="bar lv-${lv(v)}"><i style="width:${off ? 0 : v}%"></i></span><small>${(t * v / 100).toFixed(1)} GB / ${t} GB</small></div>`).join('')}</div></div>
      <section class="panel flush"><h4 class="ph">Интерфейсы</h4><table class="tbl compact"><thead><tr><th>Имя</th><th>Состояние</th><th>IP</th><th>MAC</th><th class="r">приём/с</th><th class="r">отдача/с</th><th class="r">всего приём</th><th class="r">всего отдача</th></tr></thead><tbody><tr><td class="mono"><b>${W ? 'Ethernet' : 'ens18'}</b></td><td>${badge(['ok', 'UP'])}</td><td class="mono">${s.ip}</td><td class="mono dim">52:54:00:aa:bb:cc</td><td class="mono r">${s.rx}</td><td class="mono r">${s.tx}</td><td class="mono r">9.0 GB</td><td class="mono r">3.0 GB</td></tr></tbody></table></section>
      <section class="panel"><h4>Система</h4><div class="dls">${dl('ОС', esc(s.osn))}${dl('Ядро', W ? '10.0.26100' : '6.1.0-28-amd64')}${dl('Архитектура', 'amd64')}${dl('Процессор', 'Intel(R) Xeon(R) E5-2680 · 8 vCPU')}${dl('Память', g + '.0 GB')}${dl('Uptime', s.up)}${dl('Загружен', '11 окт., 11:52')}${dl('Агент', s.ag)}${dl('Контакт', '23 нояб., 12:01')}</div></section>`;
  },
  metrics: s => `${tools(`<div class="seg sm"><button>1 ч</button><button class="on" aria-pressed="true">6 ч</button><button>24 ч</button><button>7 д</button></div>`, sp('данные каждые 10 секунд, хранятся 30 дней'))}<div class="charts">${[['CPU', s.cpu + '%', 'c', s.cpu], ['Память', s.ram + '%', 'r', s.ram], ['Диск: операции', '2.4 МБ/с', 'd', 30], ['Сеть', s.rx, 'n', 40]].map(([t, v, k, b]) => `<section class="panel"><div class="ch-h"><h4>${t}</h4><b class="mono">${s.st === 'off' ? '—' : v}</b></div>${chart(series(s.id + k + 'm', 60, b, 24))}<div class="ch-x mono dim"><span>15:00</span><span>17:00</span><span>19:00</span><span>21:00</span></div></section>`).join('')}</div>`,
  terminal: s => { const w = document.createElement('div'); w.className = 'sp-term'; w.innerHTML = tools(sp((wn(s) ? 'PowerShell' : 'bash') + ' · сеанс через Linager'), '<span class="grow"></span>', `<button class="btn sm" data-act="soon" data-m="Сниппеты настраиваются в «Настройках»">${ic('file-code', 14)}Сниппеты${ic('chevron-down', 13)}</button>`, `<button class="btn sm icon" data-act="term" data-id="${s.id}" title="В плавающее окно">${ic('maximize-2', 14)}</button>`); if (!S.sterms[s.id]) S.sterms[s.id] = mkTerm(s.id); w.appendChild(S.sterms[s.id]); return w; },
  rdp: s => SP.desk(s, 'RDP', 'rdp'), assist: s => SP.desk(s, 'Помощник', 'assist'),
  desk: (s, t, k) => `${tools(sp(t + ' · ' + s.id + ' · подключено'), '<span class="grow"></span>', rdTools(s.id), `<button class="btn sm icon" data-act="${k}" data-id="${s.id}" title="В плавающее окно">${ic('maximize-2', 14)}</button>`)}<div class="sp-rd">${desktop(s)}</div>`,
  services: s => T(['Служба', 'Описание', 'Состояние', 'Автозапуск', ''], svcRows(s).map(([n, d, ok]) => [`<span class="mono">${n}</span>`, sp(d), badge(ok ? ['ok', 'работает'] : ['crit', 'упала']), badge(['mute', 'включён']), `<span class="r-btns"><button class="btn icon sm" title="${ok ? 'Остановить' : 'Запустить'}" data-act="soon" data-m="${ok ? 'Остановка требует подтверждения' : 'Служба запущена (демо)'}">${ic(ok ? 'square' : 'play', 14)}</button><button class="btn icon sm" title="Перезапустить" data-act="confirm" data-t="Перезапуск службы" data-m="Перезапустить ${n} на ${s.id}?" data-b="Перезапустить">${ic('rotate-ccw', 14)}</button></span>`])),
  processes: s => `${tools(`<label class="field">${ic('search', 14)}<input placeholder="Фильтр по имени"></label>`, '<span class="grow"></span>', sp('обновляется каждые 3 секунды'))}` + T(['PID', 'Пользователь', 'Процесс', 'CPU ↓', 'RAM', ''], [['790', 'postgres', 'postgres: main', (s.cpu * .3).toFixed(1) + '%', '9.4%'], ['812', 'www-data', 'nginx: worker', (s.cpu * .4).toFixed(1) + '%', '3.1%'], ['655', 'root', 'linager-agent', '1.2%', '1.1%'], ['1204', 'admin', 'sshd: admin@pts/0', '0.7%', '0.4%'], ['1', 'root', 'systemd', '0.1%', '0.3%']].map(r => [`<span class="mono dim">${r[0]}</span>`, r[1], `<b>${r[2]}</b>`, `<span class="mono">${r[3]}</span>`, `<span class="mono">${r[4]}</span>`, `<button class="btn icon sm" title="Завершить" data-act="confirm" data-t="Завершить процесс" data-m="Завершить процесс ${r[2]} (PID ${r[0]})?" data-b="Завершить">${ic('x', 14)}</button>`])),
  logs: s => `${tools(`<div class="chips"><button class="chip" aria-pressed="true">Все</button><button class="chip" aria-pressed="false">Ошибки</button><button class="chip" aria-pressed="false">Предупреждения</button></div>`, `<label class="field">${ic('search', 14)}<input placeholder="Фильтр по тексту"></label>`, '<span class="grow"></span>', sp('journalctl · последние 200'))}<div class="term logv"><span class="c-d">Nov 23 12:01:44</span> sshd[1204]: Accepted publickey for admin from 10.0.0.5\n<span class="c-d">Nov 23 11:58:20</span> systemd[1]: <span class="c-g">Started</span> nginx.service\n<span class="c-d">Nov 23 11:40:03</span> nft[2210]: ruleset applied\n<span class="c-d">Nov 23 10:12:51</span> nginx[812]: <span class="c-y">warn</span> upstream response is buffered to a temporary file\n<span class="c-d">Nov 23 09:41:07</span> postgres[790]: checkpoint complete\n<span class="c-d">Nov 23 08:00:02</span> CRON[3310]: (root) CMD (/usr/local/bin/backup.sh)\n<span class="c-d">Nov 23 03:00:11</span> backup.sh[3311]: <span class="c-r">error</span> rsync: connection unexpectedly closed (retrying)\n<span class="c-d">Nov 23 03:00:41</span> backup.sh[3311]: <span class="c-g">done</span> 48 MB transferred</div>`,
  updates: s => `${tools(sp('последняя проверка 5 минут назад'), '<span class="grow"></span>', `<button class="btn sm" data-act="soon" data-m="Проверка обновлений запущена (демо)">${ic('refresh-cw', 14)}Проверить</button>`, `<button class="btn sm primary" data-act="confirm" data-t="Установка обновлений" data-m="Установить выбранные обновления на ${s.id}?" data-b="Установить">Установить выбранные</button>`)}` + T(['', 'Пакет', 'Версия', 'Источник'], [['openssl', '3.0.15-1', '3.0.16-1', 'security'], ['libssl3', '3.0.15-1', '3.0.16-1', 'security'], ['nginx', '1.22.1-9', '1.22.1-9+deb12u2', 'security'], ['linux-image-amd64', '6.1.119-1', '6.1.123-1', 'updates'], ['tzdata', '2024a-0', '2024b-0', 'updates']].map(([a, b, c, d]) => ['<label class="ck"><input type="checkbox" checked><i></i></label>', `<b class="mono">${a}</b>`, `<span class="mono dim">${b} → ${c}</span>`, badge([d === 'security' ? 'warn' : 'mute', d])]), 'compact'),
  files: s => { const w = document.createElement('div'); w.className = 'fm-page'; w.innerHTML = fmHtml(s.id, 'page'); return w; },
  disk: s => T(['Точка', 'Устройство', 'ФС', 'Размер', 'Занято', 'Свободно'], [['/', '/dev/vda1', 'ext4', '80 ГБ', s.disk], ['/var', '/dev/vdb1', 'ext4', '200 ГБ', Math.min(95, s.disk + 12)], ['/data', '/dev/vdc1', 'xfs', '1 ТБ', Math.max(5, s.disk - 20)]].map(([a, b, f, z, v]) => [`<span class="mono">${a}</span>`, `<span class="mono dim">${b}</span>`, f, z, bar(v, s.st === 'off'), `<span class="mono">${(100 - v)}%</span>`])) + `<section class="panel"><h4>Блочные устройства</h4>${dl('vda', '80 ГБ · SSD · virtio')}${dl('vdb', '200 ГБ · SSD · virtio')}${dl('vdc', '1 ТБ · HDD · virtio')}</section>`,
  network: s => T(['Интерфейс', 'Адрес', 'MAC', 'Состояние', 'Принято', 'Отправлено'], [['eth0', s.ip + '/24', '52:54:00:a1:' + (hash(s.id) & 255).toString(16).padStart(2, '0') + ':3c', 'основной'], ['eth1', '172.16.0.' + (hash(s.id) & 63) + '/24', '52:54:00:b7:11:09', 'внутренний'], ['lo', '127.0.0.1/8', '—', 'loopback']].map(([a, b, c, d]) => [`<b class="mono">${a}</b>`, `<span class="mono">${b}</span>`, `<span class="mono dim">${c}</span>`, badge(['mute', d]), `<span class="mono">${s.rx}</span>`, `<span class="mono">${s.tx}</span>`])) +
    `<section class="panel"><h4>Настройка eth0</h4><div class="form"><label>Адрес<input value="${s.ip}/24"></label><label>Шлюз<input value="10.0.1.1"></label><label>DNS<input value="10.0.1.2, 1.1.1.1"></label></div><div class="note">Изменения сети применяются с откатом: если связь с агентом пропадёт, через 30 секунд он вернёт прежние настройки.</div><div class="row-end"><button class="btn sm" data-act="soon" data-m="Черновик сохранён (демо)">Сохранить черновик</button><button class="btn sm primary" data-act="confirm" data-t="Применить настройки сети" data-m="Применить настройки eth0 с откатом через 30 секунд?" data-b="Применить">Применить с откатом</button></div></section>`,
  firewall: s => `${tools(`<div class="pol"><span>input</span>${badge(['crit', 'drop'])}<span>forward</span>${badge(['crit', 'drop'])}<span>output</span>${badge(['ok', 'accept'])}</div>`, '<span class="grow"></span>', `<button class="btn sm primary" data-act="confirm" data-t="Применить правила файрвола" data-m="Применить правила с откатом через 30 секунд?" data-b="Применить">Применить с откатом</button>`)}` + T(['№', 'Правило', 'Действие', 'Пакетов'], [['ct state established,related', 'accept', '1.2M'], ['iif lo', 'accept', '48k'], ['tcp dport 22 ip saddr 10.0.0.0/24', 'accept', '812'], ['tcp dport { 80, 443 }', 'accept', '904k'], ['icmp type echo-request', 'accept', '2.1k'], ['tcp dport 5432 ip saddr 10.0.1.0/24', 'accept', '31k']].map(([a, b, c], i) => [`<span class="mono dim">${i + 1}</span>`, `<span class="mono">${a}</span>`, badge(['ok', b]), `<span class="mono dim">${c}</span>`]), 'compact'),
  security: s => `<section class="panel flush"><h4 class="ph">Состояние</h4>${[['Порт SSH', '22', 'ok'], ['Вход root', 'запрещён', 'ok'], ['Вход по паролю', 'разрешён', 'warn'], ['Вход по ключу', 'разрешён', 'ok'], ['Пустые пароли', 'нет', 'ok'], ['fail2ban', 'активен, заблокировано 17', 'ok'], ['Автообновления', 'включены', 'ok'], ['Обновления безопасности', '3 ожидают', 'warn']].map(([a, b, k]) => `<div class="li"><span><span class="dot ${k}"></span>${a}</span><span class="dim">${b}</span></div>`).join('')}</section>` + `<h4 class="sh">Сейчас в системе</h4>` + T(['Пользователь', 'Терминал', 'Откуда', 'С', ''], [['admin', 'pts/0', '10.0.0.5', '12:01'], ['ivan.petrov', 'pts/1', '10.0.0.17', '11:48']].map(r => [`<b>${r[0]}</b>`, `<span class="mono">${r[1]}</span>`, `<span class="mono dim">${r[2]}</span>`, `<span class="mono dim">${r[3]}</span>`, `<button class="btn sm" data-act="confirm" data-t="Завершить сеанс" data-m="Завершить сеанс ${r[0]} (${r[1]})?" data-b="Завершить">Завершить</button>`])),
  users: s => T(['Пользователь', 'UID', 'Оболочка', 'Группы', 'Последний вход'], (wn(s) ? [['Administrator', '500', '—', 'Administrators', 'сегодня'], ['Guest', '501', '—', 'Guests', 'никогда']] : [['root', '0', '/bin/bash', 'root', '18 ноя 11:03'], ['admin', '1000', '/bin/bash', 'admin, sudo, docker', 'сегодня 12:01'], ['deploy', '1001', '/bin/bash', 'deploy, www-data', '22 ноя 09:12'], ['www-data', '33', '/usr/sbin/nologin', 'www-data', '—']]).map(r => [`<b>${r[0]}</b>`, `<span class="mono dim">${r[1]}</span>`, `<span class="mono">${r[2]}</span>`, r[3], sp(r[4])])),
  manage: s => `<section class="panel"><h4>Удалённый рабочий стол</h4>${sw(true, 'RDP включён', 'Порт 3389, проверка подлинности на уровне сети')}<div class="note">На Windows Home RDP недоступен; Помощник работает независимо от него.</div></section><h4 class="sh">Сеансы Windows</h4>` + T(['ID', 'Пользователь', 'Состояние', 'Тип'], [['1', 'Administrator', 'активен', 'консоль'], ['2', 'ivan', 'отключён', 'RDP']].map(r => [`<span class="mono">${r[0]}</span>`, `<b>${r[1]}</b>`, badge([r[2] === 'активен' ? 'ok' : 'mute', r[2]]), r[3]])) + `<section class="panel"><h4>Качество картинки Помощника</h4><div class="seg sm"><button>Экономный</button><button class="on" aria-pressed="true">Обычный</button><button>Высокий</button></div></section>`,
  docker: s => T(['Контейнер', 'Образ', 'Состояние', 'Порты', 'CPU', ''], [['app', 'registry/app:2.4.1', 1, '8080→80', '3.1%'], ['worker', 'registry/app:2.4.1', 1, '—', '11%'], ['redis', 'redis:7.2', 1, '6379', '0.4%'], ['migrate', 'registry/app:2.4.1', 0, '—', '—']].map(([a, b, ok, p, c]) => [`<b class="mono">${a}</b>`, `<span class="mono dim">${b}</span>`, badge(ok ? ['ok', 'работает'] : ['mute', 'завершён']), `<span class="mono">${p}</span>`, `<span class="mono">${c}</span>`, `<span class="r-btns"><button class="btn icon sm" data-act="soon" data-m="Логи контейнера" title="Логи">${ic('file-text', 14)}</button><button class="btn icon sm" data-act="confirm" data-t="Перезапуск контейнера" data-m="Перезапустить контейнер ${a}?" data-b="Перезапустить" title="Перезапустить">${ic('rotate-ccw', 14)}</button></span>`])),
};

/* ---------- floating windows ---------- */
const WK = { term: ['square-terminal', 'Терминал', 1, [740, 430]], files: ['folder', 'Файлы', 1, [780, 460]], proc: ['activity', 'Диспетчер', 1, [620, 400]], rdp: ['monitor', 'RDP', 1, [820, 540]], assist: ['app-window', 'Помощник', 1, [820, 540]], editor: ['pencil', 'Редактор', 1, [720, 480]], note: ['sticky-note', 'Заметка', 0, [420, 300]], settings: ['settings', 'Настройки', 0, [900, 580]], confirm: ['triangle-alert', 'Подтверждение', 0, [420, 176]] };
const wTitle = w => w.kind === 'settings' ? 'Настройки' : w.kind === 'confirm' ? w.opt.title : w.kind === 'editor' ? `Редактор<i>·</i><b>${w.sid}</b><i>·</i>${esc(w.opt.path.split('/').pop())}` : `${WK[w.kind][1]}<i>·</i><b>${w.sid}</b>`;
function winBody(w) {
  const s = byId(w.sid), k = w.kind;
  if (k === 'term') { w.term = mkTerm(w.sid); return w.term; }
  if (k === 'files') return filesHtml(w.sid);
  if (k === 'proc') return procHtml(w.sid);
  if (k === 'rdp' || k === 'assist') return `<div class="c-bar"><div class="c-bar-t"><span class="dot ${s.st}"></span>${WK[k][1]}<i>·</i><span class="dim">подключено</span></div><span class="grow"></span>${rdTools(w.sid)}</div><div class="rd-wrap">${desktop(s)}</div>${rdInfo}`;
  if (k === 'note') return mkNote(w);
  if (k === 'editor') { w.term = w.ed = mkEditor(w); return w.ed; }
  if (k === 'settings') return settingsBody();
  if (k === 'confirm') return `<div class="cf"><p>${esc(w.opt.text)}</p><div class="cf-b"><button class="btn sm" data-act="wclose">Отмена</button><button class="btn sm primary" data-act="cok">${esc(w.opt.ok)}</button></div></div>`;
}
function openWin(kind, sid = '', opt = {}) {
  const key = kind + ':' + sid + ':' + (opt.path || ''), ex = kind !== 'confirm' && S.wins.find(w => w.key === key);
  if (ex) { ex.min = false; ex.el.classList.remove('min'); focusWin(ex); return ex; }
  const n = S.wins.filter(w => w.kind !== 'confirm').length, [W, H] = WK[kind][3], canMax = WK[kind][2];
  const w = { id: ++winSeq, key, kind, sid, opt, min: false, max: false }, el = document.createElement('div');
  el.className = 'win kind-' + kind;
  const left = kind === 'settings' ? Math.max(8, (innerWidth - W) / 2) : kind === 'confirm' ? Math.max(8, (innerWidth - W) / 2) : 110 + n * 28;
  const top = kind === 'settings' ? 64 : kind === 'confirm' ? 140 : 70 + n * 28;
  el.style.cssText = `left:${left}px;top:${top}px;width:${W}px;height:${H}px`;
  el.innerHTML = `<div class="win-h"><span class="win-t">${ic(WK[kind][0], 14)}<span class="wt">${wTitle(w)}</span></span><span class="grow"></span>
    <button data-act="wmin" title="Свернуть">${ic('minus', 14)}</button>${canMax ? `<button data-act="wmax" title="Развернуть">${ic('maximize-2', 13)}</button>` : ''}<button data-act="wclose" class="x" title="Закрыть">${ic('x', 14)}</button></div><div class="win-b"></div>`;
  const b = $('.win-b', el), body = winBody(w);
  typeof body === 'string' ? b.innerHTML = body : b.appendChild(body);
  w.el = el; S.wins.push(w); $('#wins').appendChild(el);
  el.addEventListener('mousedown', () => focusWin(w));
  if (kind !== 'confirm') {
    el.insertAdjacentHTML('beforeend', '<div class="win-rs" title="Изменить размер"><svg viewBox="0 0 14 14" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"><path d="M12.5 4.5 4.5 12.5M12.5 8.5 8.5 12.5"/></svg></div>');
    const g = $('.win-rs', el);
    g.addEventListener('pointerdown', e => {
      if (w.max) return; e.preventDefault(); g.setPointerCapture(e.pointerId);
      const r = el.getBoundingClientRect(), x0 = e.clientX, y0 = e.clientY, cs = getComputedStyle(el), mw = parseFloat(cs.minWidth), mh = parseFloat(cs.minHeight);
      const mv = ev => { el.style.width = Math.max(mw, Math.min(innerWidth - r.left - 4, r.width + ev.clientX - x0)) + 'px'; el.style.height = Math.max(mh, Math.min(innerHeight - r.top - 4, r.height + ev.clientY - y0)) + 'px'; };
      const up = () => { g.removeEventListener('pointermove', mv); g.removeEventListener('pointerup', up); };
      g.addEventListener('pointermove', mv); g.addEventListener('pointerup', up);
    });
  }
  const h = $('.win-h', el);
  h.addEventListener('pointerdown', e => {
    if (e.target.closest('button') || w.max) return;
    const r = el.getBoundingClientRect(), dx = e.clientX - r.left, dy = e.clientY - r.top; h.setPointerCapture(e.pointerId);
    const mv = ev => { el.style.left = Math.max(-r.width + 120, Math.min(innerWidth - 120, ev.clientX - dx)) + 'px'; el.style.top = Math.max(0, Math.min(innerHeight - 40, ev.clientY - dy)) + 'px'; };
    const up = () => { h.removeEventListener('pointermove', mv); h.removeEventListener('pointerup', up); };
    h.addEventListener('pointermove', mv); h.addEventListener('pointerup', up);
  });
  if (canMax) h.addEventListener('dblclick', e => { if (!e.target.closest('button')) toggleMax(w); });
  focusWin(w); return w;
}
function focusWin(w) { w.el.style.zIndex = ++S.z; S.wins.forEach(x => x.el.classList.toggle('focus', x === w)); w.term && setTimeout(() => w.term.focusIn(), 0); dock(); }
function toggleMax(w) { w.max = !w.max; w.el.classList.toggle('max', w.max); }
function winOf(t) { return S.wins.find(x => x.el === t.closest('.win')); }
function closeWin(w) { w.el.remove(); S.wins = S.wins.filter(x => x !== w); dock(); }
function dock() {
  const d = $('#dock'); d.classList.toggle('on', S.wins.length > 0);
  const top = S.wins.filter(w => !w.min).sort((a, b) => b.el.style.zIndex - a.el.style.zIndex)[0];
  d.innerHTML = S.wins.map(w => `<button class="dock-i ${w === top ? 'on' : ''} ${w.min ? 'is-min' : ''}" data-act="dock" data-w="${w.id}">${ic(WK[w.kind][0], 13)}${w.kind === 'settings' ? 'Настройки' : w.kind === 'confirm' ? w.opt.title : w.kind === 'editor' ? w.opt.path.split('/').pop() + ' · ' + w.sid : WK[w.kind][1] + ' · ' + w.sid}</button>`).join('');
}
function procHtml(sid) {
  const s = byId(sid), P = [['790', 'postgres', s.cpu * .3, 9.4], ['812', 'nginx', s.cpu * .4, 3.1], ['1204', 'sshd', .7, .4], ['655', 'linager-agent', 1.2, 1.1], ['1', 'systemd', .1, .3]];
  return `<div class="fm"><div class="fm-bar"><div class="seg sm"><button aria-pressed="true" data-act="pseg" data-p="p">Процессы</button><button aria-pressed="false" data-act="pseg" data-p="s">Службы</button></div></div>
  <table class="tbl compact p-p"><thead><tr><th>PID</th><th>Процесс</th><th>CPU</th><th>RAM</th></tr></thead><tbody>${P.map(([a, b, c, d]) => `<tr><td class="mono dim">${a}</td><td>${b}</td><td class="mono">${c.toFixed(1)}%</td><td class="mono">${d}%</td></tr>`).join('')}</tbody></table>
  <table class="tbl compact p-s" hidden><thead><tr><th>Служба</th><th>Состояние</th></tr></thead><tbody>${svcRows(s).map(([n, , ok]) => `<tr><td>${n}</td><td>${badge(ok ? ['ok', 'работает'] : ['crit', 'остановлена'])}</td></tr>`).join('')}</tbody></table></div>`;
}

/* ---------- settings (a floating window like the others) ---------- */
const SECT = [['profile', 'user', 'Профиль'], ['users', 'users', 'Пользователи'], ['groups', 'folder-tree', 'Группы серверов'], ['snippets', 'file-code', 'Сниппеты'], ['access', 'shield-check', 'Доступ к админке'], ['connection', 'network', 'Подключение'], ['updates', 'download', 'Обновления'], ['audit', 'scroll-text', 'Журнал']];
const sw = (on, l, d = '') => `<label class="sw-row"><span><b>${l}</b>${d ? `<small>${d}</small>` : ''}</span><span class="sw"><input type="checkbox" ${on ? 'checked' : ''}><i></i></span></label>`;
const sec = {
  profile: () => `<h2>Профиль</h2><p class="lead">Учётная запись администратора и способы входа.</p>
    <div class="panel"><h4>Смена пароля</h4><div class="form"><label>Текущий пароль<input type="password" value="••••••••••"></label><label>Новый пароль<input type="password" placeholder="Не короче 12 символов"></label><label>Повторите пароль<input type="password"></label></div><div class="row-end"><button class="btn sm primary">Сменить пароль</button></div></div>
    <div class="panel"><h4>Безопасность входа</h4>${sw(true, 'Двухфакторная защита (2FA)', 'Код из приложения-аутентификатора при входе')}${sw(true, 'Passkey', 'Вход без пароля: Touch ID, ключ безопасности')}
      <div class="kv"><span>Коды восстановления</span><b>осталось 8 из 10</b><button class="btn sm">Создать новые</button></div></div>
    <div class="panel"><h4>Интерфейс</h4><div class="kv"><span>Язык</span><div class="seg sm"><button aria-pressed="true">Русский</button><button aria-pressed="false">English</button></div></div><div class="kv"><span>Тема</span><div class="seg sm" id="themeSeg"><button data-act="themeset" data-t="light">Светлая</button><button data-act="themeset" data-t="dark">Тёмная</button></div></div></div>`,
  users: () => `<div class="h-row"><div><h2>Пользователи</h2><p class="lead">Кто может входить в админку и что ему доступно.</p></div><button class="btn sm primary">${ic('plus', 14)}Добавить</button></div>
    <div class="panel flush"><table class="tbl compact"><thead><tr><th>Пользователь</th><th>Роль</th><th>2FA</th><th>Последний вход</th><th></th></tr></thead><tbody>
    ${[['admin', 'Администратор', 'info', 1, 'сейчас'], ['ivan.petrov', 'Оператор', 'mute', 1, 'сегодня, 09:12'], ['anna.k', 'Оператор', 'mute', 0, 'вчера, 17:40'], ['audit', 'Аудитор', 'mute', 1, '18 ноя, 11:03']].map(([n, r, k, f, d]) => `<tr><td><span class="ava">${n[0].toUpperCase()}</span><b>${n}</b></td><td>${badge([k, r])}</td><td>${f ? badge(['ok', 'включена']) : badge(['warn', 'нет'])}</td><td class="dim">${d}</td><td class="r"><button class="btn icon sm">${ic('ellipsis', 14)}</button></td></tr>`).join('')}</tbody></table></div>`,
  groups: () => `<h2>Группы серверов</h2><p class="lead">Группы раскладывают серверы на дашборде и в Консоли.</p><div class="panel flush">${GROUPS.map(g => `<div class="li"><span>${ic('folders', 15)}<b>${g}</b></span><span class="dim">${SV.filter(s => s.g === g).length} серверов</span><button class="btn icon sm">${ic('ellipsis', 14)}</button></div>`).join('')}</div><div class="row-end"><button class="btn sm">${ic('plus', 14)}Новая группа</button></div>`,
  snippets: () => `<h2>Сниппеты</h2><p class="lead">Команды в один клик из терминала.</p><div class="panel flush">${[['Перезапуск nginx', 'sudo systemctl restart nginx'], ['Место на дисках', 'df -h'], ['Обновить пакеты', 'sudo apt update && apt list --upgradable'], ['Последние ошибки', 'journalctl -p err -n 50 --no-pager']].map(([n, c]) => `<div class="li"><span><b>${n}</b></span><code>${c}</code><button class="btn icon sm">${ic('ellipsis', 14)}</button></div>`).join('')}</div>`,
  access: () => `<h2>Доступ к админке</h2><p class="lead">Откуда можно открыть панель управления.</p><div class="panel">${sw(true, 'Ограничить по IP', 'Остальные адреса получат отказ до ввода пароля')}<div class="chips-in">${['10.0.0.0/24', '192.168.10.0/24', '203.0.113.15'].map(x => `<span class="tagx mono">${x}${ic('x', 12)}</span>`).join('')}<button class="btn sm">${ic('plus', 14)}Добавить</button></div></div><div class="panel">${sw(true, 'Блокировать после неверных паролей', '5 попыток, блокировка на 15 минут')}${sw(false, 'Только HTTPS', 'Перенаправлять http на https')}</div>`,
  connection: () => `<h2>Подключение</h2><p class="lead">Порты, на которые подключаются агенты и приложение Linager. Изменение применяется сразу, перезапускать сервер не нужно.</p>
    <div class="panel"><h4>Сертификат</h4><div class="kv"><span>Отпечаток</span><code class="fpr">SHA256:UCpS+c3pnpiCeH4lq71BMNYKcO2A3SSJrTdSFhmTOE</code><button class="btn sm" data-act="soon" data-m="Скопировано">${ic('copy', 14)}</button></div><div class="note">Один и тот же у порта агентов и у порта приложения. Приложение запоминает его при первом подключении.</div></div>
    <div class="panel"><h4>Порты</h4><div class="form two"><label>Порт агентов<input value="9443" inputmode="numeric"></label><label><span>Порт приложения <span class="badge b-ok">слушает</span></span><input value="9444" inputmode="numeric"></label></div>
      <div class="note">Агенты переезжают на новый порт сами, старый остаётся открытым, пока все не перейдут. Новый порт нужно открыть в файрволе.</div>
      <div class="row-end"><button class="btn sm primary" data-act="confirm" data-t="Изменить порты" data-m="Порт агентов 9443 → 9500. Агенты переедут на новый порт сами." data-b="Применить">Применить</button></div></div>
    <div class="panel"><h4>Агенты переезжают на новый порт</h4><p style="font-size:12.5px;margin-bottom:8px">С порта 9443 на порт 9500: перешло 9 из 12.</p><div class="bar lv-ok" style="height:6px"><i style="width:75%"></i></div>
      <div class="kv" style="margin-top:10px"><span>mail-01</span><span class="dim">не в сети, перейдёт при подключении</span></div><div class="kv"><span>backup-nas <span class="dim">версия 0.8.4</span></span><span class="dim">старая версия, сам не перейдёт</span></div><div class="kv"><span>office-pc</span><span class="dim">ждёт</span></div>
      <div class="row-end"><button class="btn sm danger" data-act="confirm" data-t="Закрыть старый порт" data-m="Закрыть порт 9443? Агенты, которые не перешли, потеряют связь с сервером." data-b="Закрыть">Закрыть старый порт</button></div></div>`,
  updates: () => `<h2>Обновления Linager</h2><p class="lead">Сервер и агенты обновляются отдельно.</p><div class="panel"><div class="upd"><div><small>Установлена</small><b class="mono">0.9.1</b></div><div class="arrow">${ic('arrow-right', 16)}</div><div><small>Доступна</small><b class="mono acc">0.9.2</b></div><span class="grow"></span><button class="btn sm primary">${ic('download', 14)}Обновить сервер</button></div><ul class="chg"><li>Читаемый журнал аудита</li><li>Фильтры в Консоли по ОС и статусу</li><li>Исправлена прокрутка списка в плавающих окнах</li></ul></div>
    <div class="panel"><h4>Агенты</h4><div class="kv"><span>Актуальны</span><b>11 из 12</b></div><div class="kv"><span>Требуют обновления</span><b>backup-nas <i class="dim">(0.8.4)</i></b><button class="btn sm">Обновить</button></div></div>`,
  audit: () => `<h2>Журнал</h2><p class="lead">Все действия администраторов.</p><div class="chips" style="margin-bottom:12px"><button class="chip" aria-pressed="true">Все</button><button class="chip" aria-pressed="false">Вход</button><button class="chip" aria-pressed="false">Серверы</button><button class="chip" aria-pressed="false">Файрвол</button></div>
    <div class="panel flush"><table class="tbl compact"><thead><tr><th>Время</th><th>Кто</th><th>Действие</th><th>IP</th></tr></thead><tbody>${[['12:01:44', 'admin', 'Открыт терминал · web-01', '10.0.0.5'], ['11:58:20', 'ivan.petrov', 'Перезапущена служба nginx · web-02', '10.0.0.17'], ['11:40:03', 'admin', 'Применены правила файрвола · db-primary', '10.0.0.5'], ['10:12:51', 'anna.k', 'Вход выполнен', '192.168.10.8'], ['09:12:09', 'ivan.petrov', 'Вход выполнен (2FA)', '10.0.0.17'], ['вчера', 'admin', 'Создан пользователь audit', '10.0.0.5']].map(r => `<tr><td class="mono dim">${r[0]}</td><td><b>${r[1]}</b></td><td>${r[2]}</td><td class="mono dim">${r[3]}</td></tr>`).join('')}</tbody></table></div>`,
};
function settingsBody() {
  const k = S.settingsTab;
  return `<div class="st"><aside class="d-side">${SECT.map(([id, i, t]) => `<button class="d-nav ${id === k ? 'on' : ''}" data-act="settings" data-s="${id}">${ic(i, 15)}<span>${t}</span>${id === 'updates' ? '<em class="cnt">3</em>' : ''}</button>`).join('')}<span class="grow"></span><button class="d-nav" data-act="soon" data-m="Выход из админки">${ic('log-out', 15)}<span>Выйти</span></button></aside><section class="d-main"><div class="d-body">${sec[k]()}</div></section></div>`;
}
function openSettings(k = 'profile') {
  S.settingsTab = k;
  const w = openWin('settings');
  $('.win-b', w.el).innerHTML = settingsBody(); syncThemeSeg();
}

/* ---------- theme ---------- */
function setTheme(t) { document.documentElement.dataset.theme = t; try { localStorage.setItem('lg-theme', t); } catch (e) { } const b = $('[data-act=theme]'); if (b) b.innerHTML = ic(t === 'dark' ? 'sun' : 'moon', 16); syncThemeSeg(); }
function syncThemeSeg() { $$('#themeSeg button').forEach(b => b.setAttribute('aria-pressed', b.dataset.t === document.documentElement.dataset.theme)); }

/* ---------- events ---------- */
document.addEventListener('click', e => {
  const t = e.target.closest('[data-act]');
  if (!e.target.closest('.pop') && !(t && t.dataset.act === 'selopen')) closePop();
  if (!t) return; const d = t.dataset;
  switch (d.act) {
    case 'view': setView(d.v); break;
    case 'server': S.sid = d.id; S.stab = 'overview'; setView('server'); break;
    case 'stab': S.stab = d.t; renderServer(); break;
    case 'sgrp': S.sgc[d.g] = !S.sgc[d.g]; renderServer(); break;
    case 'spc': S.spc = !S.spc; renderServer(); break;
    case 'fm-tog': fmUpdate(t, st => { st.open.has(d.p) ? st.open.delete(d.p) : st.open.add(d.p); }); break;
    case 'fm-go': fmUpdate(t, st => { st.path = d.p; fmOpenTo(st, d.p); }); break;
    case 'fm-edit': { e.stopPropagation(); openWin('editor', FM[t.closest('[data-fm]').dataset.fm].sid, { path: d.p }); break; }
    case 'ed-save': winOf(t).ed.save(); break;
    case 'filter': S.filter = d.f; updGrid(); break;
    case 'layout': S.layout = d.l; updGrid(); break;
    case 'term': case 'files': case 'proc': case 'rdp': case 'assist': case 'note': openWin(d.act, d.id); break;
    case 'win': openWin(d.k, d.id); break;
    case 'soon': toast(d.m || 'В макете не реализовано'); break;
    case 'settings': openSettings(d.s); break;
    case 'confirm': openWin('confirm', '', { title: d.t, text: d.m, ok: d.b }); break;
    case 'cok': { const w = winOf(t); closeWin(w); toast('Выполнено (демо): ' + w.opt.title); break; }
    case 'theme': setTheme(document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark'); break;
    case 'themeset': setTheme(d.t); break;
    case 'selopen': openPop(t); break;
    case 'selpick': SELS[d.sel].set(d.v); closePop(); break;
    case 'mode': S.mode = d.m; renderConsole(); break;
    case 'grp': S.collapsed[d.g] = !S.collapsed[d.g]; renderConsole(); break;
    case 'csrv': openTab(d.id, S.mode); renderConsole(); break;
    case 'tab': S.active = +d.t; S.mode = curTab().mode; renderConsole(); break;
    case 'tabx': { e.stopPropagation(); const i = S.tabs.findIndex(x => x.id === +d.t); S.tabs.splice(i, 1); if (S.active === +d.t) S.active = S.tabs.length ? S.tabs[Math.max(0, i - 1)].id : null; renderConsole(); break; }
    case 'wmin': { const w = winOf(t); w.min = true; w.el.classList.add('min'); dock(); break; }
    case 'wmax': toggleMax(winOf(t)); break;
    case 'wclose': closeWin(winOf(t)); break;
    case 'dock': { const w = S.wins.find(x => x.id === +d.w); const topZ = Math.max(...S.wins.filter(x => !x.min).map(x => +x.el.style.zIndex), 0);
      if (w.min) { w.min = false; w.el.classList.remove('min'); focusWin(w); } else if (+w.el.style.zIndex === topZ) { w.min = true; w.el.classList.add('min'); dock(); } else focusWin(w); break; }
    case 'pseg': { const f = t.closest('.fm'); $$('[data-act=pseg]', f).forEach(b => b.setAttribute('aria-pressed', b === t)); $('.p-p', f).hidden = d.p !== 'p'; $('.p-s', f).hidden = d.p !== 's'; break; }
  }
});
document.addEventListener('keydown', e => { if (e.key === 'Escape') closePop(); });

window.LG = {
  init(c) {
    let th = c.theme; try { th = localStorage.getItem('lg-theme') || th; } catch (e) { }
    shell(); setTheme(th);
    const p = new URLSearchParams(location.search), v = p.get('view');
    if (p.get('server')) { S.sid = p.get('server'); S.stab = p.get('tab') || 'overview'; setView('server'); }
    else setView(v === 'console' ? 'console' : 'dash');
    if (p.get('layout')) { S.layout = p.get('layout'); if (S.view === 'dash') updGrid(); }
    if (p.get('win')) { const [k, id] = p.get('win').split(':'); openWin(k, id); }
    if (p.get('settings')) openSettings(p.get('settings'));
  },
};
})();
