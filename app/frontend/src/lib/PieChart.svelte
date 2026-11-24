<script lang="ts">
  import { fmtBytes } from './status';

  export interface PiePart {
    id: string;
    name: string;
    value: number;
    color: string;
  }

  let { parts, size = 150, legend = true, showShare = false }: { parts: PiePart[]; size?: number; /** A table next to the chart; a row lifts its sector. */ legend?: boolean; /** A column with the share of each part. */ showShare?: boolean } = $props();

  const LIFT = 8;
  let hovered = $state<string | null>(null);
  let tip = $state<{ x: number; y: number } | null>(null);
  let host = $state<HTMLDivElement | undefined>(undefined);

  const total = $derived(parts.reduce((n, p) => n + p.value, 0));
  const r = $derived(size / 2);

  /** The path of every sector and the angle of its middle (the direction it is lifted in). */
  const sectors = $derived.by(() => {
    const out: { part: PiePart; d: string; mid: number }[] = [];
    if (total <= 0) return out;
    let a0 = -Math.PI / 2;
    const list = parts.filter((p) => p.value > 0);
    for (const part of list) {
      const a1 = a0 + (part.value / total) * 2 * Math.PI;
      const big = a1 - a0 > Math.PI ? 1 : 0;
      const p = (a: number): string => `${(r + r * Math.cos(a)).toFixed(2)} ${(r + r * Math.sin(a)).toFixed(2)}`;
      const d = list.length === 1 ? `M${r} 0 A${r} ${r} 0 1 1 ${r - 0.01} 0 Z` : `M${r} ${r} L${p(a0)} A${r} ${r} 0 ${big} 1 ${p(a1)} Z`;
      out.push({ part, d, mid: (a0 + a1) / 2 });
      a0 = a1;
    }
    return out;
  });
  const lifted = $derived(sectors.find((s) => s.part.id === hovered) ?? null);
  const share = (v: number): string => (total > 0 ? `${Math.round((v / total) * 100)}%` : '—');

  function place(e: PointerEvent): { x: number; y: number } {
    const box = host?.getBoundingClientRect();
    return { x: e.clientX - (box?.left ?? 0) + 14, y: e.clientY - (box?.top ?? 0) - 34 };
  }
  function leave(): void {
    hovered = null;
    tip = null;
  }
</script>

<div class="pie-block">
  <div class="piewrap" bind:this={host} role="img" aria-label={parts.map((p) => `${p.name}: ${fmtBytes(p.value)}`).join(', ')} onpointerleave={leave} onpointercancel={leave}>
    <svg class="pie" width={size} height={size} viewBox="0 0 {size} {size}" aria-hidden="true">
      {#each sectors as s (s.part.id)}
        <!-- the sector under the pointer is never moved: a copy is lifted above it, so the events stay stable -->
        <path role="presentation" d={s.d} fill={s.part.color} style:opacity={hovered === s.part.id ? 0 : undefined}
          onpointerenter={(e) => { hovered = s.part.id; tip = place(e); }} onpointermove={(e) => { if (hovered === s.part.id) tip = place(e); }} onpointerleave={leave} />
      {/each}
      {#if lifted}
        <g pointer-events="none"><path d={lifted.d} fill={lifted.part.color} style:transform="translate({(Math.cos(lifted.mid) * LIFT).toFixed(1)}px, {(Math.sin(lifted.mid) * LIFT).toFixed(1)}px)" /></g>
      {/if}
    </svg>
    {#if lifted && tip}<div class="pietip" style:left="{tip.x}px" style:top="{tip.y}px"><b>{lifted.part.name}</b> · {fmtBytes(lifted.part.value)} · {share(lifted.part.value)}</div>{/if}
  </div>
  {#if legend}
    <table class="legend"><tbody>
      {#each parts as p (p.id)}
        <tr class:on={hovered === p.id} onpointerenter={() => (hovered = p.id)} onpointerleave={leave}>
          <td><i class="sw" style:background={p.color}></i>{p.name}</td>
          <td class="num mono">{fmtBytes(p.value)}</td>
          {#if showShare}<td class="num dim">{share(p.value)}</td>{/if}
        </tr>
      {/each}
    </tbody></table>
  {/if}
</div>
