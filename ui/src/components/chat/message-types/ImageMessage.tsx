import { useState } from "react"
import { mediaApi } from "@/api/media"
import { cn } from "@/lib/utils"
import { Image as ImageIcon } from "lucide-react"
import { useImagePreviewStore } from "@/stores/image-preview"
import { usePlatform } from "@/hooks/usePlatform"

interface ImageMessageProps {
  id?: string | number // Message ID required for preview
  md5?: string
  path?: string
  content?: string // Sometimes content contains the URL
}

export function ImageMessage({ id, md5, path, content }: ImageMessageProps) {
  const [error, setError] = useState(false)
  const [loaded, setLoaded] = useState(false)
  const openPreview = useImagePreviewStore(state => state.openPreview)
  // 只用来解释失败原因，不再用于「是否尝试加载」
  const { imagesUnavailable } = usePlatform()
  const imageUrl = content || (md5 ? mediaApi.getImageUrl(md5, path) : "")
  const thumbUrl = content || (md5 ? mediaApi.getThumbnailUrl(md5, path) : "")

  // 注意：macOS 上并不是所有图片都解不开。2025 年 5 月之前的图片以明文
  // `_M.dat` 存放，可以正常显示；之后微信改成加密存储，那些才解不出来。
  // 所以这里不再按平台一刀切，而是让每张图各自尝试加载，
  // 后端对确实解不开的返回 415，落到下面的 [图片] 占位。

  if (!imageUrl) {
    return (
      <div className="flex items-center justify-center w-32 h-32 bg-muted rounded-lg text-muted-foreground">
        <ImageIcon className="w-8 h-8" />
        <span className="ml-2 text-xs">无效图片</span>
      </div>
    )
  }

  return (
    <div className="relative overflow-hidden rounded-lg">
      {!loaded && !error && (
        <div className="flex items-center justify-center bg-muted animate-pulse w-32 h-32 rounded-lg">
          <ImageIcon className="w-6 h-6 text-muted-foreground/50" />
        </div>
      )}
      
      {error ? (
        imagesUnavailable ? (
          // 拿不到图片密钥时（推导失败等），加密图片确实解不开，
          // 按微信原生的样子显示 [图片] 并说明原因。
          <div
            className="inline-flex items-center gap-1.5 rounded-lg bg-muted/60 px-2.5 py-1.5 text-muted-foreground"
            title="图片密钥不可用，这张加密图片无法解出"
          >
            <ImageIcon className="h-3.5 w-3.5" />
            <span className="text-xs">[图片]</span>
          </div>
        ) : (
          <div
            className="flex flex-col items-center justify-center w-32 h-32 bg-muted text-muted-foreground p-2 rounded-lg"
            title="本地没有这张图的文件 —— 微信只在你打开过时才下载原图"
          >
            <ImageIcon className="w-8 h-8 mb-1" />
            <span className="text-[10px]">图片未下载</span>
          </div>
        )
      ) : (
        <img
          src={thumbUrl} // Use thumbnail first
          alt="Image"
          className={cn(
            "block max-w-[240px] max-h-[240px] w-auto h-auto cursor-zoom-in transition-opacity duration-300 rounded-lg",
            loaded ? "opacity-100" : "opacity-0"
          )}
          onLoad={() => setLoaded(true)}
          onError={() => {
            setError(true)
            setLoaded(true) // Stop pulse
          }}
          onClick={() => id && openPreview(id)}
        />
      )}
    </div>
  )
}
