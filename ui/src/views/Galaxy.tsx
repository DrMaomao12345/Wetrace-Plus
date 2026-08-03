import { useEffect, useRef, useState } from 'react'
import { galaxyApi } from '@/api/galaxy'
import type { GalaxyNode, RelationshipGraph, GalaxyTimeline } from '@/api/galaxy'

const TYPE_COLORS: Record<string, string> = {
  friend: '#eab308', family: '#22c55e', teacher: '#a855f7', classmate: '#3b82f6',
  work: '#f97316', online: '#06b6d4', service: '#6b7280', stranger: '#6b7280',
}
const TYPE_LABEL: Record<string, string> = {
  friend: '朋友', family: '家人', teacher: '老师', classmate: '同学',
  work: '工作', online: '网络', service: '服务', stranger: '陌生',
}
const STATUS_LABEL: Record<string, string> = {
  active: '当前活跃', low_freq: '低频维持', faded: '历史重要 · 已淡出', resumed: '重新联系',
}

function hx(hex: string) { const m = hex.replace('#', ''); return { r: parseInt(m.slice(0, 2), 16), g: parseInt(m.slice(2, 4), 16), b: parseInt(m.slice(4, 6), 16) } }
function hexA(hex: string, a: number) { const { r, g, b } = hx(hex); return `rgba(${r},${g},${b},${a})` }
function dim(hex: string, f: number) { const { r, g, b } = hx(hex); return `rgb(${Math.round(r * f)},${Math.round(g * f)},${Math.round(b * f)})` }
function fmtDate(t: number) { return t ? new Date(t * 1000).toISOString().slice(0, 10) : '—' }

export default function Galaxy() {
  const [loading, setLoading] = useState(true)
  const [needProfile, setNeedProfile] = useState(false)
  const [graph, setGraph] = useState<RelationshipGraph | null>(null)
  const [selected, setSelected] = useState<GalaxyNode | null>(null)
  const [rebuilding, setRebuilding] = useState(false)
  const [view, setView] = useState<'galaxy' | 'timeline' | 'memory'>('galaxy')
  const [anim, setAnim] = useState(true)
  const [by, setBy] = useState(2003)
  const [bm, setBm] = useState(9)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const viewRef = useRef({ scale: 1, panX: 0, panY: 0 })
  const animRef = useRef(true)

  const loadGraph = async () => {
    setLoading(true)
    try { setGraph(await galaxyApi.getGraph()); setNeedProfile(false) }
    catch { setNeedProfile(true) }
    finally { setLoading(false) }
  }
  useEffect(() => {
    (async () => {
      try {
        const p = await galaxyApi.getProfile()
        if (!p.exists) { setNeedProfile(true); setLoading(false); return }
        await loadGraph()
      } catch { setLoading(false) }
    })()
  }, [])

  const submitProfile = async () => {
    setRebuilding(true)
    try {
      await galaxyApi.updateProfile(by, bm)
      setNeedProfile(false)
      setGraph(await galaxyApi.rebuild(50))
    } finally { setRebuilding(false) }
  }
  const rebuild = async () => {
    setRebuilding(true)
    try { setGraph(await galaxyApi.rebuild(50)); setSelected(null) } finally { setRebuilding(false) }
  }
  // Phase4 保存单个联系人的手动修正,然后刷新图(服务端读取时套用修正)
  const patchContact = async (patch: Record<string, unknown>) => {
    if (!selected) return
    const id = selected.contact_id
    try {
      await galaxyApi.patchContact(id, patch)
      const g = await galaxyApi.getGraph()
      setGraph(g)
      setSelected(g.nodes.find((n) => n.contact_id === id) || null)
    } catch { /* ignore */ }
  }

  useEffect(() => {
    if (!graph) return
    const canvas = canvasRef.current; if (!canvas) return
    const ctx = canvas.getContext('2d'); if (!ctx) return
    const dpr = window.devicePixelRatio || 1
    const maxR = Math.max(1, ...graph.nodes.map(n => Math.hypot(n.x, n.y)))
    const pts = graph.nodes.map(n => ({ n, sx: 0, sy: 0 }))
    let raf = 0

    const resize = () => { const r = canvas.getBoundingClientRect(); canvas.width = r.width * dpr; canvas.height = r.height * dpr }
    resize()
    const ro = new ResizeObserver(resize); ro.observe(canvas)

    const render = (t: number) => {
      const W = canvas.width, H = canvas.height
      ctx.fillStyle = '#0a0a12'; ctx.fillRect(0, 0, W, H)
      const cx = W / 2 + viewRef.current.panX * dpr
      const cy = H / 2 + viewRef.current.panY * dpr
      const scale = (Math.min(W, H) / 2 / (maxR * 1.18)) * viewRef.current.scale

      // 等距同心圆环：帮助感知节点与中心的距离(=亲密度层级)
      const ringN = 5
      const ringMax = maxR * scale
      ctx.save()
      ctx.setLineDash([3 * dpr, 5 * dpr])
      ctx.lineWidth = 1.2 * dpr
      for (let k = 1; k <= ringN; k++) {
        ctx.beginPath(); ctx.arc(cx, cy, (ringMax * k) / ringN, 0, Math.PI * 2)
        ctx.strokeStyle = `rgba(150,170,255,${0.22 - (k - 1) * 0.028})`
        ctx.stroke()
      }
      ctx.setLineDash([])
      ctx.restore()

      for (const it of pts) {
        const n = it.n
        it.sx = cx + n.x * scale; it.sy = cy + n.y * scale
        const color = TYPE_COLORS[n.relationship_type] || '#6b7280'
        ctx.beginPath(); ctx.moveTo(cx, cy); ctx.lineTo(it.sx, it.sy)
        ctx.strokeStyle = hexA(color, (n.status === 'faded' ? 0.05 : 0.08) + 0.22 * n.current_temperature / 100)
        ctx.lineWidth = (0.4 + 2 * n.relationship_depth / 100) * dpr
        ctx.stroke()
      }
      for (const it of pts) {
        const n = it.n, px = it.sx, py = it.sy
        const color = TYPE_COLORS[n.relationship_type] || '#6b7280'
        const faded = n.status === 'faded'
        const pulse = (animRef.current && n.recent_activity > 20) ? 1 + 0.05 * Math.sin(t / (620 - n.recent_activity * 4)) : 1
        const rNode = (6 + 5 * n.composite_intimacy / 100) * dpr * pulse
        const glowR = rNode + (5 + 20 * n.relationship_depth / 100) * dpr
        const glowA = (0.12 + 0.5 * n.relationship_depth / 100) * (faded ? 0.35 : 0.4 + 0.6 * n.current_temperature / 100)
        const g = ctx.createRadialGradient(px, py, rNode * 0.4, px, py, glowR)
        g.addColorStop(0, hexA(color, glowA)); g.addColorStop(1, hexA(color, 0))
        ctx.fillStyle = g; ctx.beginPath(); ctx.arc(px, py, glowR, 0, Math.PI * 2); ctx.fill()

        const rings = Math.min(4, n.life_stage_count)
        for (let r = 0; r < rings; r++) {
          ctx.beginPath(); ctx.arc(px, py, rNode + (3 + r * 3) * dpr, 0, Math.PI * 2)
          ctx.strokeStyle = hexA(color, n.confidence < 60 ? 0.18 : 0.38); ctx.lineWidth = 1 * dpr
          ctx.setLineDash(n.confidence < 60 ? [3 * dpr, 3 * dpr] : [])
          ctx.stroke(); ctx.setLineDash([])
        }
        const bright = faded ? 0.35 : 0.55 + 0.45 * n.current_temperature / 100
        ctx.beginPath(); ctx.arc(px, py, rNode, 0, Math.PI * 2)
        ctx.fillStyle = faded ? '#3a3a44' : dim(color, bright); ctx.fill()
        if (n.manual_important) { ctx.strokeStyle = '#fbbf24'; ctx.lineWidth = 2 * dpr; ctx.stroke() }
        if (selected?.contact_id === n.contact_id) { ctx.strokeStyle = '#fff'; ctx.lineWidth = 2 * dpr; ctx.stroke() }
        ctx.fillStyle = '#fff'; ctx.font = `${9 * dpr}px sans-serif`; ctx.textAlign = 'center'; ctx.textBaseline = 'middle'
        ctx.fillText((n.display_name || '?').slice(0, 1), px, py)
        if (n.composite_intimacy >= 68 || selected?.contact_id === n.contact_id) {
          ctx.fillStyle = hexA('#ffffff', 0.72); ctx.font = `${10 * dpr}px sans-serif`; ctx.textBaseline = 'top'
          ctx.fillText(n.display_name.slice(0, 8), px, py + rNode + 3 * dpr)
        }
      }
      ctx.beginPath(); ctx.arc(cx, cy, 13 * dpr, 0, Math.PI * 2); ctx.fillStyle = '#fff'; ctx.fill()
      ctx.fillStyle = '#0a0a12'; ctx.font = `bold ${11 * dpr}px sans-serif`; ctx.textAlign = 'center'; ctx.textBaseline = 'middle'
      ctx.fillText('我', cx, cy)
      raf = requestAnimationFrame(render)
    }
    raf = requestAnimationFrame(render)

    const onClick = (e: MouseEvent) => {
      const r = canvas.getBoundingClientRect()
      const mx = (e.clientX - r.left) * dpr, my = (e.clientY - r.top) * dpr
      let hit: GalaxyNode | null = null, hd = Infinity
      for (const it of pts) { const d = Math.hypot(it.sx - mx, it.sy - my); if (d < 22 * dpr && d < hd) { hd = d; hit = it.n } }
      setSelected(hit)
    }
    const onWheel = (e: WheelEvent) => { e.preventDefault(); const v = viewRef.current; v.scale = Math.max(0.4, Math.min(4, v.scale * (e.deltaY < 0 ? 1.1 : 0.9))) }
    let drag = false, lx = 0, ly = 0
    const onDown = (e: MouseEvent) => { drag = true; lx = e.clientX; ly = e.clientY }
    const onMove = (e: MouseEvent) => { if (!drag) return; const v = viewRef.current; v.panX += e.clientX - lx; v.panY += e.clientY - ly; lx = e.clientX; ly = e.clientY }
    const onUp = () => { drag = false }
    canvas.addEventListener('click', onClick)
    canvas.addEventListener('wheel', onWheel, { passive: false })
    canvas.addEventListener('mousedown', onDown)
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    return () => {
      cancelAnimationFrame(raf); ro.disconnect()
      canvas.removeEventListener('click', onClick); canvas.removeEventListener('wheel', onWheel); canvas.removeEventListener('mousedown', onDown)
      window.removeEventListener('mousemove', onMove); window.removeEventListener('mouseup', onUp)
    }
  }, [graph, selected])

  if (needProfile) {
    const years = Array.from({ length: 66 }, (_, i) => 2015 - i)
    return (
      <div className="h-full flex items-center justify-center p-6">
        <div className="w-full max-w-md rounded-2xl border border-border bg-card p-6 shadow-sm">
          <h2 className="text-lg font-bold">关系星图 · 初始化</h2>
          <p className="mt-1 text-sm text-muted-foreground">输入你的出生年月，用于推断人生阶段（小学/初中/高中/大学/工作），把联系人放到对应的星域。之后可修改。</p>
          <div className="mt-4 flex items-center gap-2">
            <select className="h-9 rounded-lg border border-border bg-background px-2 text-sm" value={by} onChange={e => setBy(+e.target.value)}>
              {years.map(y => <option key={y} value={y}>{y} 年</option>)}
            </select>
            <select className="h-9 rounded-lg border border-border bg-background px-2 text-sm" value={bm} onChange={e => setBm(+e.target.value)}>
              {Array.from({ length: 12 }, (_, i) => i + 1).map(m => <option key={m} value={m}>{m} 月</option>)}
            </select>
          </div>
          <button onClick={submitProfile} disabled={rebuilding}
            className="mt-5 w-full h-10 rounded-xl bg-primary text-primary-foreground text-sm font-medium disabled:opacity-60">
            {rebuilding ? '正在生成星图…' : '生成关系星图'}
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="relative h-full w-full overflow-hidden bg-[#0a0a12]">
      <canvas ref={canvasRef} className="absolute inset-0 h-full w-full cursor-grab active:cursor-grabbing" />
      {view === 'timeline' && graph?.timeline && (
        <TimelineView tl={graph.timeline} nodes={graph.nodes} selectedId={selected?.contact_id} onPick={(id) => setSelected(graph.nodes.find((n) => n.contact_id === id) || null)} />
      )}
      {view === 'memory' && graph?.timeline && (
        <MemoryView graph={graph} onPick={(id) => setSelected(graph.nodes.find((n) => n.contact_id === id) || null)} />
      )}
      {/* 顶部工具条 */}
      <div className="absolute left-4 top-4 z-10 flex items-center gap-3 rounded-xl bg-black/40 px-3 py-2 backdrop-blur">
        <span className="text-sm font-semibold text-white">关系星图</span>
        {graph && <span className="text-xs text-white/60">Top {graph.nodes.length} / 共 {graph.total_count} 人</span>}
        <div className="flex rounded-lg bg-white/10 p-0.5 text-xs">
          <button onClick={() => setView('galaxy')} className={`rounded px-2 py-0.5 ${view === 'galaxy' ? 'bg-white/25 text-white' : 'text-white/60'}`}>星图</button>
          <button onClick={() => setView('timeline')} className={`rounded px-2 py-0.5 ${view === 'timeline' ? 'bg-white/25 text-white' : 'text-white/60'}`}>时间轴</button>
          <button onClick={() => setView('memory')} className={`rounded px-2 py-0.5 ${view === 'memory' ? 'bg-white/25 text-white' : 'text-white/60'}`}>回忆</button>
        </div>
        <button onClick={rebuild} disabled={rebuilding} className="rounded-lg bg-white/10 px-2 py-1 text-xs text-white hover:bg-white/20 disabled:opacity-50">
          {rebuilding ? '重算中…' : '重新分析'}
        </button>
        <button onClick={() => setNeedProfile(true)} className="rounded-lg bg-white/10 px-2 py-1 text-xs text-white hover:bg-white/20">改出生年月</button>
        <button onClick={() => { animRef.current = !anim; setAnim(!anim) }} className="rounded-lg bg-white/10 px-2 py-1 text-xs text-white hover:bg-white/20">{anim ? '动画开' : '动画关'}</button>
      </div>
      {loading && <div className="absolute inset-0 z-10 flex items-center justify-center text-sm text-white/70">加载星图…</div>}
      {/* 图例 */}
      <div className="absolute bottom-4 left-4 z-10 flex flex-wrap gap-x-3 gap-y-1 rounded-xl bg-black/40 px-3 py-2 text-[11px] text-white/70 backdrop-blur">
        {Object.entries(TYPE_LABEL).filter(([k]) => k !== 'stranger').map(([k, v]) => (
          <span key={k} className="flex items-center gap-1"><i className="inline-block h-2 w-2 rounded-full" style={{ background: TYPE_COLORS[k] }} />{v}</span>
        ))}
        <span className="text-white/40">离中心越近越亲密 · 光晕=历史深度 · 亮度=当前温度</span>
      </div>

      {/* 详情卡 */}
      {selected && (
        <div className="absolute right-4 top-4 bottom-4 z-10 w-72 overflow-y-auto rounded-2xl bg-black/60 p-4 text-white backdrop-blur">
          <button onClick={() => setSelected(null)} className="float-right text-white/50 hover:text-white">✕</button>
          <div className="flex items-center gap-2">
            <span className="inline-block h-3 w-3 rounded-full" style={{ background: TYPE_COLORS[selected.relationship_type] }} />
            <h3 className="text-base font-bold">{selected.display_name}</h3>
          </div>
          <div className="mt-1 text-xs text-white/60">{selected.relationship_label} · {STATUS_LABEL[selected.status] || selected.status}</div>
          <div className="mt-2 flex gap-1.5 text-[11px]">
            {view !== 'timeline' && <button onClick={() => setView('timeline')} className="rounded bg-white/10 px-2 py-1 hover:bg-white/20">查看时间轨迹 →</button>}
            {view !== 'galaxy' && <button onClick={() => setView('galaxy')} className="rounded bg-white/10 px-2 py-1 hover:bg-white/20">← 在星图查看</button>}
          </div>

          <div className="mt-4 space-y-2">
            {([['历史深度', selected.relationship_depth], ['当前温度', selected.current_temperature], ['综合亲密度', selected.composite_intimacy], ['陪伴持续', selected.continuity_score], ['双向交流', selected.reciprocity_score]] as [string, number][]).map(([label, v]) => (
              <div key={label}>
                <div className="flex justify-between text-[11px] text-white/60"><span>{label}</span><span>{v}</span></div>
                <div className="mt-0.5 h-1.5 w-full rounded-full bg-white/10">
                  <div className="h-full rounded-full" style={{ width: `${v}%`, background: TYPE_COLORS[selected.relationship_type] }} />
                </div>
              </div>
            ))}
          </div>

          <div className="mt-4 grid grid-cols-2 gap-2 text-[11px] text-white/70">
            <div>主要阶段<div className="text-white">{selected.main_life_stage}</div></div>
            <div>跨越阶段<div className="text-white">{selected.life_stage_count} 个</div></div>
            <div>消息总数<div className="text-white">{selected.total_messages.toLocaleString()}</div></div>
            <div>活跃月份<div className="text-white">{selected.active_months} 个月</div></div>
            <div>首次聊天<div className="text-white">{fmtDate(selected.first_time)}</div></div>
            <div>最后聊天<div className="text-white">{fmtDate(selected.last_time)}</div></div>
            <div>置信度<div className="text-white">{selected.confidence}%</div></div>
          </div>

          <div className="mt-4">
            <div className="text-[11px] font-medium text-white/60">分析依据</div>
            <ul className="mt-1 space-y-1 text-[11px] text-white/70">
              {selected.evidence.map((e, i) => <li key={i}>· {e}</li>)}
            </ul>
          </div>

          <div className="mt-4 border-t border-white/10 pt-3">
            <div className="mb-2 text-[11px] font-medium text-white/60">人工修正（你的判断优先，重建不丢）</div>
            <div className="space-y-2 text-xs">
              <label className="flex items-center gap-2">
                <span className="w-10 shrink-0 text-white/50">关系</span>
                <select value={selected.relationship_type} onChange={(e) => patchContact({ relationship_type: e.target.value })} className="flex-1 rounded bg-white/10 px-2 py-1">
                  {Object.entries(TYPE_LABEL).map(([k, v]) => <option key={k} value={k} className="bg-[#0a0a12]">{v}</option>)}
                </select>
              </label>
              <label className="flex items-center gap-2">
                <span className="w-10 shrink-0 text-white/50">阶段</span>
                <select value={selected.main_life_stage} onChange={(e) => patchContact({ main_life_stage: e.target.value })} className="flex-1 rounded bg-white/10 px-2 py-1">
                  {(graph?.profile?.life_stages || []).map((s) => <option key={s.name} value={s.name} className="bg-[#0a0a12]">{s.name}</option>)}
                </select>
              </label>
              <div className="flex flex-wrap gap-1.5">
                <button onClick={() => patchContact({ manual_important: !selected.manual_important })} className={`rounded px-2 py-1 ${selected.manual_important ? 'bg-amber-500/30 text-amber-300' : 'bg-white/10'}`}>{selected.manual_important ? '★ 重要' : '☆ 标记重要'}</button>
                <button onClick={() => patchContact({ hidden: true })} className="rounded bg-white/10 px-2 py-1 hover:bg-red-500/30">隐藏</button>
                <button onClick={() => patchContact({ reset: true })} className="rounded bg-white/10 px-2 py-1">恢复默认</button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ── 功能11 陪伴时间轴视图 ────────────────────────────────────
function TimelineView({ tl, nodes, selectedId, onPick }: { tl: GalaxyTimeline; nodes: GalaxyNode[]; selectedId?: string; onPick: (id: string) => void }) {
  const months = tl.months
  const cw = 14 // 每月格宽 px
  const trackW = months.length * cw
  const [sort, setSort] = useState<'first' | 'intimacy' | 'name'>('first')
  const selRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => { selRef.current?.scrollIntoView({ block: 'center', behavior: 'smooth' }) }, [selectedId])
  const intimacy = new Map(nodes.map((n) => [n.contact_id, n.composite_intimacy]))
  const contacts = [...tl.contacts].sort((a, b) => {
    if (sort === 'name') return a.display_name.localeCompare(b.display_name, 'zh')
    if (sort === 'intimacy') return (intimacy.get(b.contact_id) || 0) - (intimacy.get(a.contact_id) || 0)
    return a.first_month < b.first_month ? -1 : 1
  })
  const cell = (s: number) => (s <= 0 ? 'transparent' : `rgba(236,72,153,${0.12 + (0.78 * s) / 100})`)
  const heat = (s: number) => `rgba(129,140,248,${0.08 + (0.9 * s) / 100})`
  const yearMarks: { i: number; y: string }[] = []
  months.forEach((m, i) => {
    const y = m.slice(0, 4)
    if (i === 0 || months[i - 1].slice(0, 4) !== y) yearMarks.push({ i, y })
  })

  return (
    <div className="absolute inset-0 z-[5] overflow-auto bg-[#0a0a12] pt-16 pb-6 text-white">
      <div className="px-4" style={{ minWidth: trackW + 176 }}>
        {/* 年份轴 */}
        <div className="sticky top-0 z-10 mb-1 flex items-end bg-[#0a0a12] pb-1">
          <div className="flex w-40 shrink-0 items-center gap-1 text-[11px] text-white/50">
            <span>排序</span>
            <select value={sort} onChange={(e) => setSort(e.target.value as 'first' | 'intimacy' | 'name')} className="rounded bg-white/10 px-1 py-0.5 text-white/80">
              <option value="first" className="bg-[#0a0a12]">首次出现</option>
              <option value="intimacy" className="bg-[#0a0a12]">亲密度</option>
              <option value="name" className="bg-[#0a0a12]">姓名</option>
            </select>
          </div>
          <div className="relative h-4" style={{ width: trackW }}>
            {yearMarks.map(({ i, y }) => (
              <span key={y} className="absolute text-[11px] text-white/50" style={{ left: i * cw }}>{y}</span>
            ))}
          </div>
        </div>
        {/* 全局热力带 */}
        <div className="mb-2 flex items-center">
          <div className="w-40 shrink-0 text-[11px] text-white/70">总体陪伴热力</div>
          <div className="flex" style={{ width: trackW }}>
            {months.map((m) => {
              const g = tl.global_heat.find((x) => x.month === m)
              return (
                <div key={m} title={g ? `${m}　活跃 ${g.active_relationships} 人 / 强关系 ${g.strong_relationships} 人 / ${g.message_count} 条` : m}
                  style={{ width: cw - 1, height: 20, marginRight: 1, background: g ? heat(g.heat_score) : 'transparent', borderRadius: 2 }} />
              )
            })}
          </div>
        </div>
        {/* 每人轨迹 */}
        <div className="space-y-0.5">
          {contacts.map((c) => {
            const smap = new Map(c.segments.map((s) => [s.month, s]))
            return (
              <div key={c.contact_id} ref={c.contact_id === selectedId ? selRef : undefined} className={`flex cursor-pointer items-center rounded ${c.contact_id === selectedId ? 'bg-white/15 ring-1 ring-white/30' : 'hover:bg-white/5'}`} onClick={() => onPick(c.contact_id)}>
                <div className="w-40 shrink-0 truncate pr-2 text-[12px]">
                  <span className="mr-1 inline-block h-2 w-2 rounded-full align-middle" style={{ background: TYPE_COLORS[c.relationship_type] || '#888' }} />
                  {c.display_name}
                </div>
                <div className="flex" style={{ width: trackW, height: 16 }}>
                  {months.map((m) => {
                    const s = smap.get(m)
                    return (
                      <div key={m} title={s ? `${m}　强度 ${s.strength} / ${s.message_count} 条` : m}
                        style={{ width: cw - 1, height: 12, marginRight: 1, marginTop: 2, background: s ? cell(s.strength) : 'transparent', borderRadius: 2 }} />
                    )
                  })}
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

function monthDiff(a: string, b: string) {
  const [ay, am] = a.split('-').map(Number)
  const [by, bm] = b.split('-').map(Number)
  return (by - ay) * 12 + (bm - am)
}

// ── Phase3 人生回忆模式：年度关系回顾(§13) ───────────────────
function MemoryView({ graph, onPick }: { graph: RelationshipGraph; onPick: (id: string) => void }) {
  const tl = graph.timeline!
  const years = Array.from(new Set(tl.months.map((m) => m.slice(0, 4)))).sort()
  const [year, setYear] = useState(years[years.length - 1] || String(new Date().getFullYear()))
  const nodeMap = new Map(graph.nodes.map((n) => [n.contact_id, n]))

  const rows = tl.contacts.map((c) => {
    const yseg = c.segments.filter((s) => s.month.startsWith(year))
    const yStr = yseg.reduce((a, s) => a + s.strength, 0)
    const node = nodeMap.get(c.contact_id)
    const firstY = c.first_month.slice(0, 4)
    let resumed = false
    if (yseg.length) {
      const before = c.segments.filter((s) => s.month < yseg[0].month)
      if (before.length && monthDiff(before[before.length - 1].month, yseg[0].month) >= 6) resumed = true
    }
    return { c, node, yStr, firstY, active: yseg.length > 0, resumed }
  })

  const active = rows.filter((r) => r.active)
  const top = [...active].sort((a, b) => b.yStr - a.yStr).slice(0, 8)
  const newly = active.filter((r) => r.firstY === year)
  const continued = active.filter((r) => r.firstY < year && !r.resumed)
  const resumedList = active.filter((r) => r.resumed)
  const faded = rows.filter((r) => !r.active && r.firstY < year && (r.node?.relationship_depth || 0) >= 50)

  const stage = (graph.profile?.life_stages || []).find((s) => {
    const st = (s.start || '0000').slice(0, 4)
    const en = s.end ? s.end.slice(0, 4) : '9999'
    return year >= st && year <= en
  })
  const kw = new Set<string>()
  if (stage) kw.add(stage.name)
  top.forEach((r) => { if (r.node) { kw.add(TYPE_LABEL[r.node.relationship_type] || ''); if (r.node.main_life_stage) kw.add(r.node.main_life_stage) } })

  const nameList = (arr: typeof rows) => arr.slice(0, 12).map((r) => r.c.display_name).join('、') || '无'
  const quad: [string, typeof rows, string][] = [
    ['新增重要关系', newly, 'text-emerald-400'],
    ['持续陪伴', continued, 'text-sky-400'],
    ['重新恢复联系', resumedList, 'text-amber-400'],
    ['逐渐淡出', faded, 'text-white/50'],
  ]

  return (
    <div className="absolute inset-0 z-[5] overflow-auto bg-[#0a0a12] px-4 pt-16 pb-6 text-white">
      <div className="mx-auto max-w-2xl">
        <div className="mb-4 flex items-center gap-2">
          <h2 className="text-lg font-bold">{year} 年关系回顾</h2>
          <select value={year} onChange={(e) => setYear(e.target.value)} className="ml-auto rounded bg-white/10 px-2 py-1 text-sm">
            {years.map((y) => <option key={y} value={y} className="bg-[#0a0a12]">{y}</option>)}
          </select>
        </div>

        <div className="rounded-2xl bg-white/5 p-4">
          <div className="mb-2 text-sm font-medium text-white/80">这一年陪伴你最多的人</div>
          <div className="space-y-1.5">
            {top.map((r, i) => (
              <div key={r.c.contact_id} className="flex cursor-pointer items-center gap-2" onClick={() => onPick(r.c.contact_id)}>
                <span className="w-5 text-right text-xs text-white/40">{i + 1}</span>
                <span className="h-2 w-2 shrink-0 rounded-full" style={{ background: TYPE_COLORS[r.node?.relationship_type || ''] || '#888' }} />
                <span className="w-28 shrink-0 truncate text-sm">{r.c.display_name}</span>
                <span className="truncate text-[11px] text-white/40">{r.node?.relationship_label}</span>
                <div className="ml-auto h-1.5 w-24 shrink-0 rounded-full bg-white/10">
                  <div className="h-full rounded-full bg-pink-400" style={{ width: `${Math.min(100, (r.yStr / (top[0]?.yStr || 1)) * 100)}%` }} />
                </div>
              </div>
            ))}
            {top.length === 0 && <div className="text-xs text-white/40">这一年没有有效聊天记录</div>}
          </div>
        </div>

        <div className="mt-4 grid grid-cols-2 gap-3">
          {quad.map(([label, arr, cls]) => (
            <div key={label} className="rounded-xl bg-white/5 p-3">
              <div className="flex items-baseline gap-2"><span className={`text-2xl font-bold ${cls}`}>{arr.length}</span><span className="text-xs text-white/60">{label}</span></div>
              <div className="mt-1 text-[11px] leading-snug text-white/40">{nameList(arr)}</div>
            </div>
          ))}
        </div>

        <div className="mt-4 flex flex-wrap gap-2 text-xs">
          <span className="rounded-full bg-white/10 px-3 py-1">主要阶段：{stage?.name || '—'}</span>
          {[...kw].filter(Boolean).map((k) => <span key={k} className="rounded-full bg-white/10 px-3 py-1 text-white/70">{k}</span>)}
        </div>
      </div>
    </div>
  )
}
