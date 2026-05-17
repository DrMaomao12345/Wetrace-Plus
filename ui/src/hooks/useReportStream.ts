import { useState, useRef, useCallback } from "react"
import type { AnnualReport, TZSegment } from "@/api/report"
import { getApiBaseUrl } from "@/lib/request"

export type StreamStep =
  | "overview"
  | "top_contacts"
  | "monthly_trend"
  | "past_years_avg"
  | "weekday_dist"
  | "hourly_dist"
  | "message_types"
  | "highlights"

export interface StreamState {
  status: "idle" | "running" | "done" | "error"
  current: number     // 已完成步数
  total: number       // 总步数
  step: StreamStep | "" // 当前步骤名
  partial: Partial<AnnualReport>  // 已到达的部分数据
  loadedSteps: Set<StreamStep>    // 已完成的步骤集合，用于渲染骨架
  fullReport: AnnualReport | null // 完整数据（done 后）
  params: StreamParams | null     // 本次流绑定的参数快照（用于缓存写回时取正确的 sig）
  error: string | null
}

const initial: StreamState = {
  status: "idle",
  current: 0,
  total: 8,
  step: "",
  partial: {},
  loadedSteps: new Set(),
  fullReport: null,
  params: null,
  error: null,
}

export interface StreamParams {
  year: number
  defaultTzOffset: number
  tzSegments: TZSegment[]
  excludeTalkers: string[]
}

/**
 * 流式拉取年度报告。返回 state + start() + reset()。
 */
export function useReportStream() {
  const [state, setState] = useState<StreamState>(initial)
  const abortRef = useRef<AbortController | null>(null)

  const reset = useCallback(() => {
    abortRef.current?.abort()
    abortRef.current = null
    setState(initial)
  }, [])

  const start = useCallback(async (params: StreamParams) => {
    abortRef.current?.abort()
    const controller = new AbortController()
    abortRef.current = controller

    setState({ ...initial, status: "running", params })

    try {
      const url = `${getApiBaseUrl()}/api/v1/report/annual/stream`
      const headers: Record<string, string> = { "Content-Type": "application/json" }
      const token = localStorage.getItem("auth_token")
      if (token) headers["X-Auth-Token"] = token
      const res = await fetch(url, {
        method: "POST",
        headers,
        signal: controller.signal,
        body: JSON.stringify({
          year: params.year,
          default_tz_offset: params.defaultTzOffset,
          tz_segments: params.tzSegments,
          exclude_talkers: params.excludeTalkers,
        }),
      })
      if (!res.ok || !res.body) {
        throw new Error(`HTTP ${res.status}`)
      }

      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ""

      let sawDone = false
      while (true) {
        // 该流已被新的 startStream 中止 → 立即退出，不处理任何遗留事件
        if (controller.signal.aborted) {
          console.debug("[ReportStream] aborted, dropping events")
          return
        }
        const { done, value } = await reader.read()
        if (done) break
        // double-check：read 后又被中止
        if (controller.signal.aborted) {
          console.debug("[ReportStream] aborted after read, dropping events")
          return
        }
        buffer += decoder.decode(value, { stream: true })
        // 按行切分
        const lines = buffer.split("\n")
        buffer = lines.pop() || ""
        for (const line of lines) {
          if (controller.signal.aborted) return
          if (!line.trim()) continue
          let evt: any
          try { evt = JSON.parse(line) } catch (e) {
            console.warn("[ReportStream] JSON 解析失败", line.slice(0, 200), e)
            continue
          }
          // 调试：确认事件确实流式到达
          console.debug("[ReportStream]", evt.type, evt.step ?? "", `${evt.current ?? ""}/${evt.total ?? ""}`)

          if (evt.type === "section") {
            setState((prev) => {
              const newPartial = { ...prev.partial }
              const newLoaded = new Set(prev.loadedSteps)
              newLoaded.add(evt.step as StreamStep)
              switch (evt.step as StreamStep) {
                case "overview":
                  newPartial.overview = evt.data
                  break
                case "top_contacts":
                  newPartial.top_contacts = evt.data
                  break
                case "monthly_trend":
                  newPartial.monthly_trend = evt.data
                  break
                case "past_years_avg":
                  newPartial.past_years_monthly_avg = evt.data
                  break
                case "weekday_dist":
                  newPartial.weekday_distribution = evt.data
                  break
                case "hourly_dist":
                  newPartial.hourly_distribution = evt.data
                  break
                case "message_types":
                  newPartial.message_types = evt.data
                  break
                case "highlights":
                  if (evt.data) {
                    newPartial.highlights = evt.data.highlights
                    newPartial.overview_deltas = evt.data.overview_deltas
                  }
                  break
              }
              return {
                ...prev,
                current: evt.current,
                total: evt.total,
                step: evt.step,
                partial: newPartial,
                loadedSteps: newLoaded,
              }
            })
          } else if (evt.type === "done") {
            sawDone = true
            setState((prev) => ({
              ...prev,
              status: "done",
              fullReport: evt.report,
              current: prev.total,
              // done 后所有步都视为完成
              loadedSteps: new Set(["overview", "top_contacts", "monthly_trend", "past_years_avg", "weekday_dist", "hourly_dist", "message_types", "highlights"] as StreamStep[]),
            }))
          } else if (evt.type === "error") {
            sawDone = true
            setState((prev) => ({ ...prev, status: "error", error: evt.error }))
          }
        }
      }
      // 如果流结束了但没收到 done，视为后端中途异常
      if (!sawDone) {
        console.error("[ReportStream] 流提前结束，未收到 done 事件")
        setState((prev) => prev.status === "running"
          ? { ...prev, status: "error", error: "服务端中途断开，未完成生成" }
          : prev
        )
      }
    } catch (e: any) {
      if (e?.name === "AbortError") return
      setState((prev) => ({ ...prev, status: "error", error: e?.message || "stream failed" }))
    }
  }, [])

  return { state, start, reset }
}
