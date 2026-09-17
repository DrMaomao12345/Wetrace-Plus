import { Fragment } from "react"

/**
 * 把关键词高亮成 <em>，其余部分当纯文本渲染。
 *
 * 以前搜索页用 dangerouslySetInnerHTML 直接塞聊天内容 —— 任何人发一条
 * `<img src=x onerror=…>`，你一搜到它，脚本就在应用里执行，能调全部接口。
 * 这里全程走 React 文本节点，内容里的尖括号永远只是字符。
 */
export function HighlightText({ text, keyword, className }: {
  text: string | null | undefined
  keyword?: string
  className?: string
}) {
  const content = text ?? ""
  const kw = keyword?.trim()
  if (!kw) return <>{content}</>

  const lower = content.toLocaleLowerCase()
  const needle = kw.toLocaleLowerCase()
  const parts: { text: string; hit: boolean }[] = []
  let from = 0
  for (let i = lower.indexOf(needle); i !== -1; i = lower.indexOf(needle, i + needle.length)) {
    if (i > from) parts.push({ text: content.slice(from, i), hit: false })
    parts.push({ text: content.slice(i, i + needle.length), hit: true })
    from = i + needle.length
  }
  if (from < content.length) parts.push({ text: content.slice(from), hit: false })

  return (
    <>
      {parts.map((p, i) =>
        p.hit
          ? <em key={i} className={className ?? "rounded bg-yellow-200 px-0.5 not-italic dark:bg-yellow-800"}>{p.text}</em>
          : <Fragment key={i}>{p.text}</Fragment>
      )}
    </>
  )
}
