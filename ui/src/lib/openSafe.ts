/**
 * 在新标签页打开链接，只接受 http(s)。
 *
 * 链接卡片的地址来自消息 XML，也就是来自任何给你发过消息的人。
 * `window.open("javascript:…")` 会在本应用的源里执行脚本（点一下就中招），
 * 不带 noopener 的话，被打开的网页还能通过 window.opener 把这个标签页导航走。
 */
export function openSafe(url: string | null | undefined) {
  if (!url) return
  let target: URL
  try {
    target = new URL(url, window.location.origin)
  } catch {
    return
  }
  if (target.protocol !== "http:" && target.protocol !== "https:") return
  window.open(target.toString(), "_blank", "noopener,noreferrer")
}
