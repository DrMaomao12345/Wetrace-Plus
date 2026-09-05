/**
 * 月度序列的**唯一口径**：哪几个月是「真实存在」的。
 *
 * 图上的 0 有两种完全不同的含义：
 *
 *   · **真 0** —— 那个月确实一条没聊。该画，那是信息。
 *   · **假 0** —— 那个月压根不存在：要么第一条消息还没发生（联系人还没加上、
 *     存档还没开始），要么这个月还没到。
 *
 * 后端只按 GROUP BY 返回有数据的月份；前端补满 1~12 之后，补出来的假 0 会被
 * 当成真值画出来 —— 开头是一条贴着 X 轴的平线，年底是一段假的「跌到 0」。
 *
 * 这套判断以前在 MonthlyChart 和 AnnualReport 里各写了一份，代价是同一个漏洞
 * 得修两次（AnnualReport 先裁了右边、漏了左边，隔一次提交才补上）。现在只有这份。
 *
 * 口径定义 —— **真实区间 = [第一条消息所在的月, 当前月]**：
 *   - 左边界之前，数据尚不存在
 *   - 右边界之后，时间尚未发生
 *   - 区间**之内**一律是真值，包括结尾那段沉默：不聊了，0 就是 0
 *
 * 后端 `web/export/export_monthly.go` 的 `monthWindowOf` 是同一条规则的 Go 版
 * （导出 CSV/XLSX 用），改这边记得同步改那边。
 */

/** 把 (年, 月) 压成一个能直接比较、直接自增的整数。 */
export function ym(year: number, month: number): number {
  return year * 12 + month
}

/** 「现在」是哪个年月。按浏览器时区算，与 api/insights.ts 的 tz() 同口径。 */
export function nowYM(now: Date = new Date()): number {
  return ym(now.getFullYear(), now.getMonth() + 1)
}

/** 真实存在的月份区间，闭区间。 */
export interface MonthWindow {
  first: number
  last: number
}


/**
 * 从「有数据的月份」推出真实区间。
 *
 * 当月算**已经开始**，照常显示 —— 哪怕只过了几天。这会让当月的点天然偏低，
 * 那是事实，不是缺陷。
 */
export function monthWindowOf(
  points: Array<{ year: number; month: number; count: number }>,
  now: Date = new Date(),
): MonthWindow {
  let first = Infinity
  for (const p of points) {
    if (p.count <= 0 || p.month < 1 || p.month > 12) continue
    const k = ym(p.year, p.month)
    if (k < first) first = k
  }
  // 一条数据都没有时**不裁左边**：没有「第一条消息」可言，也就没有假 0 要挡。
  // 让图老实画一条贴地的 0 线，比整片空白更说明问题（右边界照裁，未来仍是未来）。
  return { first: first === Infinity ? Number.NEGATIVE_INFINITY : first, last: nowYM(now) }
}

/** 某个年月是否落在真实区间内。落在外面的该给 null，不是 0。 */
export function isRealMonth(k: number, w: MonthWindow): boolean {
  return k >= w.first && k <= w.last
}

/**
 * 单条曲线在某个月的取值：区间内返回计数（真 0 也返回 0），区间外返回 null。
 *
 * recharts 默认 `connectNulls=false`，null 会让线断开、点不画；
 * Tooltip 的 `filterNull` 默认 true，也不会把这些月份列进去。
 */
export function monthValue(k: number, w: MonthWindow, count: number): number | null {
  return isRealMonth(k, w) ? count : null
}
