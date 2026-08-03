import type { RelationshipGraph } from '@/api/galaxy'

export interface YearReviewPerson { id: string; name: string; label: string; type: string; strength: number }
export interface YearReview {
  year: string
  top: YearReviewPerson[]
  newly: string[]
  continued: string[]
  resumed: string[]
  faded: string[]
  stage?: string
  keywords: string[]
  activeMonths: number
}

function monthGap(a: string, b: string) {
  const [ay, am] = a.split('-').map(Number)
  const [by, bm] = b.split('-').map(Number)
  return (by - ay) * 12 + (bm - am)
}

// computeYearReview 从已缓存的关系星图(节点 + 时间轴)算出某一年的关系回顾。
// 纯前端、无 AI、复用时间轴月度强度(§13)。
export function computeYearReview(graph: RelationshipGraph, year: string): YearReview | null {
  const tl = graph.timeline
  if (!tl) return null
  const nodeMap = new Map(graph.nodes.map((n) => [n.contact_id, n]))

  const rows = tl.contacts.map((c) => {
    const yseg = c.segments.filter((s) => s.month.startsWith(year))
    const yStr = yseg.reduce((a, s) => a + s.strength, 0)
    const node = nodeMap.get(c.contact_id)
    const firstY = c.first_month.slice(0, 4)
    let resumed = false
    if (yseg.length) {
      const before = c.segments.filter((s) => s.month < yseg[0].month)
      if (before.length && monthGap(before[before.length - 1].month, yseg[0].month) >= 6) resumed = true
    }
    return { c, node, yStr, firstY, active: yseg.length > 0, resumed }
  })

  const active = rows.filter((r) => r.active)
  const names = (arr: typeof rows) => arr.map((r) => r.c.display_name)
  const top: YearReviewPerson[] = [...active].sort((a, b) => b.yStr - a.yStr).slice(0, 5).map((r) => ({
    id: r.c.contact_id, name: r.c.display_name, label: r.node?.relationship_label || '', type: r.node?.relationship_type || '', strength: r.yStr,
  }))

  const stage = (graph.profile?.life_stages || []).find((s) => {
    const st = (s.start || '0000').slice(0, 4)
    const en = s.end ? s.end.slice(0, 4) : '9999'
    return year >= st && year <= en
  })?.name

  const activeMonths = tl.months.filter((m) => m.startsWith(year) && rows.some((r) => r.c.segments.some((s) => s.month === m))).length

  const kw = new Set<string>()
  if (stage) kw.add(stage)
  top.forEach((t) => { const s = nodeMap.get(t.id)?.main_life_stage; if (s) kw.add(s) })

  return {
    year,
    top,
    newly: names(active.filter((r) => r.firstY === year)),
    continued: names(active.filter((r) => r.firstY < year && !r.resumed)),
    resumed: names(active.filter((r) => r.resumed)),
    faded: names(rows.filter((r) => !r.active && r.firstY < year && (r.node?.relationship_depth || 0) >= 50)),
    stage,
    keywords: [...kw],
    activeMonths,
  }
}
