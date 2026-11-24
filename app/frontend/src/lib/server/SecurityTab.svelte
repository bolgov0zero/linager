<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi2, type SecState, type ServerDetail } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import { errorText } from '../errors';
  import Icon from '../Icon.svelte';
  import { t, i18n } from '../i18n.svelte';
  import { barHeights, groupChecks, hintKey, issueCount, keyLabel, postureChecks, worstTone, type Check } from '../sec-util';
  import F2bCard from './F2bCard.svelte';
  import SessionsList from './SessionsList.svelte';

  let { s }: { s: ServerDetail } = $props();

  let sec = $state<SecState | null>(null);
  let error = $state('');
  let note = $state<{ text: string; bad: boolean } | null>(null);
  let noteTimer: ReturnType<typeof setTimeout> | undefined;
  let banning = $state<string | null>(null);

  const sid = $derived(s.id);
  const online = $derived(s.online);
  const unreachable = $derived(!online);

  async function load(): Promise<void> {
    try {
      sec = await serverApi2.security(sid);
      error = '';
    } catch (e) {
      error = errorText(e);
    }
  }
  onMount(() => {
    void load();
    const i = setInterval(() => void load(), 60_000); // the agent answers from a one minute cache
    return () => clearInterval(i);
  });

  function say(text: string, bad = false): void {
    note = { text, bad };
    clearTimeout(noteTimer);
    noteTimer = setTimeout(() => (note = null), bad ? 8000 : 4000);
  }

  const checks = $derived(sec ? postureChecks(sec, s.sample) : []);
  const tone = $derived(worstTone(checks));
  const issues = $derived(issueCount(checks));
  const groups = $derived(groupChecks(checks));
  const verdict = $derived(issues === 0 ? t('sec.state.ok') : t(`sec.state.issues.${new Intl.PluralRules(i18n.locale).select(issues)}`, { n: issues }));
  const heights = $derived(sec ? barHeights(sec.failures.by_day) : []);
  const bannedIps = $derived(new Set((sec?.fail2ban.bans ?? []).map((b) => b.ip)));
  const canBan = $derived(!!sec && sec.fail2ban.installed && sec.fail2ban.active);
  const when = (n: number): string => new Date(n * 1000).toLocaleString(undefined, { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });
  const method = (m: string): string => {
    const key = `sec.method.${m}`;
    const text = t(key);
    return text === key ? m : text;
  };
  function dayLabel(i: number, count: number): string {
    if (!sec) return '';
    return new Date((sec.generated_at - (count - 1 - i) * 86400) * 1000).toLocaleDateString(i18n.locale, { weekday: 'short' });
  }
  const mark = (c: Check): string => (c.tone === 'ok' ? 'circle-check' : c.tone === 'muted' ? 'info' : 'triangle-alert');

  async function ban(ip: string): Promise<void> {
    if (banning) return;
    banning = ip;
    try {
      await serverApi2.securityBan(sid, ip, 0);
      say(t('sec.f2b.banned.done', { ip }));
      await load();
    } catch (e) {
      say(errorText(e), true);
    } finally {
      banning = null;
    }
  }

  async function removeKey(k: SecState['keys'][number]): Promise<void> {
    if (!sec) return;
    const mine = sec.keys.filter((x) => x.user === k.user).length;
    const warn = mine > 1 ? '' : sec.ssh.password_auth === 'no' ? t('sec.keys.warnLockout', { user: k.user }) : t('sec.keys.warnLast', { user: k.user });
    const ok = await confirmAsk({ title: t('sec.keys.remove'), text: `${t('sec.keys.removeText', { fp: k.fingerprint, user: k.user })} ${warn} ${t('sec.keys.removeNote')}`.replace(/\s+/g, ' ').trim(), ok: t('sec.keys.removeGo'), danger: true });
    if (!ok) return;
    try {
      await serverApi2.securityKeyRemove(sid, { user: k.user, file: k.file, fingerprint: k.fingerprint });
      say(t('sec.keys.removed'));
      await load();
    } catch (e) {
      say(errorText(e), true);
    }
  }
</script>

{#snippet checkRows(list: Check[])}
  {#each list as c (c.id)}
    <div class="chk-row">
      <span class="{c.tone === 'crit' ? 'crit-t' : c.tone === 'warn' ? 'warn-t' : c.tone === 'ok' ? 'ok-t' : 'dim'}"><Icon name={mark(c)} size={15} /></span>
      <span style="width:190px">{t(c.label)}</span>
      <b style="font-weight:500">{t(c.value, c.vars)}</b>
      {#if hintKey(c)}<span class="hint">{t(hintKey(c)!)}</span>{/if}
    </div>
  {/each}
{/snippet}

{#if error && !sec}<div class="err">{error}</div>{:else if !sec}<p class="dim">{t('dash.loading')}</p>{:else}
  <div class="sec-page">
    {#if note}<div class="fm-note" class:bad={note.bad}><span>{note.text}</span></div>{/if}

    <section class="panel">
      <div class="verdict"><span class="{tone === 'crit' ? 'crit-t' : tone === 'warn' ? 'warn-t' : 'ok-t'}"><Icon name={tone === 'ok' ? 'circle-check' : 'triangle-alert'} size={16} /></span>{verdict}<span class="grow"></span>
        <button class="btn sm icon" type="button" title={t('sp.refresh')} onclick={load}><Icon name="refresh-cw" size={14} /></button></div>
      <div class="sh" style="margin-top:8px">SSH</div>
      {@render checkRows(groups.ssh)}
      {#if groups.system.length}<div class="sh" style="margin-top:10px">{t('sec.state.system')}</div>{@render checkRows(groups.system)}{/if}
    </section>

    <section class="panel">
      <h4 class="ph">{t('sec.sessions')}</h4>
      <SessionsList serverId={sid} sessions={sec.sessions} {online} onchanged={load} />
    </section>

    <F2bCard serverId={sid} {sec} {unreachable} onchanged={load} />

    <section class="panel">
      <h4 class="ph">{t('sec.failures', { n: sec.period_days })}</h4>
      <div class="row" style="align-items:flex-end;gap:24px">
        <div><div style="font-size:26px;font-weight:650">{sec.failures.total.toLocaleString()}</div><div class="dim">{t('sec.failures.sub', { addresses: sec.failures.addresses, invalid: sec.failures.invalid_user_attempts })}</div></div>
        <div class="bars grow" role="img" aria-label={t('sec.failures.byDay')}>
          {#each sec.failures.by_day as n, i (i)}<div class="bc" title="{dayLabel(i, sec.failures.by_day.length)}: {n}"><span>{n}</span><i style="height:{heights[i]}%"></i><span>{dayLabel(i, sec.failures.by_day.length)}</span></div>{/each}
        </div>
      </div>
      {#if sec.failures.total === 0}<p class="dim">{t('sec.failures.none')}</p>{:else}
        <div class="two" style="margin-top:12px;align-items:start">
          <div class="tw"><table class="tbl compact">
            <thead><tr><th>{t('sec.col.ip')}</th><th class="r">{t('sec.col.attempts')}</th><th class="r">{t('sec.col.usersTried')}</th><th>{t('sec.col.last')}</th>{#if canBan}<th></th>{/if}</tr></thead>
            <tbody>
              {#each sec.failures.top_ips as r (r.ip)}
                <tr><td class="mono">{r.ip}</td><td class="mono r">{r.count.toLocaleString()}</td><td class="mono r">{r.users}</td><td class="dim">{when(r.last)}</td>
                  {#if canBan}<td class="r">{#if bannedIps.has(r.ip)}<span class="badge b-crit">{t('sec.f2b.bannedPill')}</span>{:else if r.ip !== sec.self_ip}<button class="btn sm" type="button" disabled={unreachable || banning !== null} onclick={() => ban(r.ip)}>{t('sec.f2b.ban')}</button>{/if}</td>{/if}</tr>
              {/each}
            </tbody>
          </table></div>
          <div class="tw"><table class="tbl compact">
            <thead><tr><th>{t('sec.col.user')}</th><th class="r">{t('sec.col.attempts')}</th><th></th></tr></thead>
            <tbody>{#each sec.failures.top_users as r (r.user)}<tr><td class="mono">{r.user || '—'}</td><td class="mono r">{r.count.toLocaleString()}</td><td>{#if r.invalid}<span class="badge b-mute">{t('sec.badge.noUser')}</span>{/if}</td></tr>{/each}</tbody>
          </table></div>
        </div>
      {/if}
    </section>

    <section class="panel">
      <h4 class="ph">{t('sec.logins')}</h4>
      {#if sec.logins.length === 0}<p class="dim">{t('sec.logins.none')}</p>{:else}
        <div class="tw"><table class="tbl compact">
          <thead><tr><th>{t('sec.col.time')}</th><th>{t('sec.col.user')}</th><th>{t('sec.col.from')}</th><th>{t('sec.col.method')}</th><th>{t('sec.col.notes')}</th></tr></thead>
          <tbody>{#each sec.logins as l, i (i)}
            <tr><td class="dim">{when(l.time)}</td><td class="mono">{l.user}</td><td class="mono">{l.ip}</td><td>{method(l.method)}</td>
              <td>{#if l.prior_failures > 0}<span class="badge b-warn" title={t('sec.badge.priorHint')}>{t('sec.badge.prior', { n: l.prior_failures })}</span>{/if}</td></tr>
          {/each}</tbody>
        </table></div>
      {/if}
    </section>

    <section class="panel">
      <h4 class="ph">{t('sec.keys')}</h4>
      <p class="dim">{t('sec.keys.hint')}</p>
      {#if sec.keys.length === 0}<p class="dim">{t('sec.keys.none')}</p>{:else}
        <div class="tw"><table class="tbl compact">
          <thead><tr><th>{t('sec.col.user')}</th><th>{t('sec.col.keyType')}</th><th>{t('sec.col.fingerprint')}</th><th>{t('sec.col.comment')}</th><th>{t('sec.col.fileChanged')}</th><th></th></tr></thead>
          <tbody>{#each sec.keys as k, i (i)}
            <tr><td class="mono"><b>{k.user}</b></td>
              <td>{keyLabel(k.type, k.bits)}{#if k.weak} <span class="badge b-crit">{t('sec.badge.weak')}</span>{/if}{#if k.restricted} <span class="badge b-mute">{t('sec.badge.restricted')}</span>{/if}</td>
              <td class="mono dim" style="word-break:break-all">{k.fingerprint}</td><td class="dim">{k.comment || '—'}</td><td class="dim">{k.mtime ? when(k.mtime) : '—'}</td>
              <td class="r"><button class="btn sm danger" type="button" disabled={unreachable} onclick={() => removeKey(k)}>{t('sec.keys.remove')}</button></td></tr>
          {/each}</tbody>
        </table></div>
      {/if}
    </section>
  </div>
{/if}
