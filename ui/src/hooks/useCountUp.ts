import { useEffect, useRef, useState } from "react"

/**
 * 数字爬升动画 hook：从当前显示值平滑滚动到 target，约 duration 毫秒
 */
export function useCountUp(target: number, duration: number = 800): number {
  const [value, setValue] = useState<number>(0)
  const fromRef = useRef<number>(0)
  const startRef = useRef<number>(0)
  const rafRef = useRef<number>(0)

  useEffect(() => {
    if (typeof target !== "number" || isNaN(target)) return
    fromRef.current = value
    startRef.current = performance.now()

    const tick = (now: number) => {
      const elapsed = now - startRef.current
      const t = Math.min(1, elapsed / duration)
      // ease-out cubic
      const eased = 1 - Math.pow(1 - t, 3)
      const next = fromRef.current + (target - fromRef.current) * eased
      setValue(next)
      if (t < 1) {
        rafRef.current = requestAnimationFrame(tick)
      } else {
        setValue(target)
      }
    }

    cancelAnimationFrame(rafRef.current)
    rafRef.current = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(rafRef.current)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target, duration])

  return Math.round(value)
}
