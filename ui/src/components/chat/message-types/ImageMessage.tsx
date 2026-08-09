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
  const { imagesUnavailable } = usePlatform()

  const imageUrl = content || (md5 ? mediaApi.getImageUrl(md5, path) : "")
  const thumbUrl = content || (md5 ? mediaApi.getThumbnailUrl(md5, path) : "")

  // 图片密钥拿不到时，加载必然失败。与其让每条消息都去请求一遍再显示「加载失败」
  // （既浪费请求又让人以为是网络问题），不如直接按微信原生的样子显示 [图片]。
  if (imagesUnavailable) {
    return (
      <div
        className="inline-flex items-center gap-1.5 rounded-lg bg-muted/60 px-2.5 py-1.5 text-muted-foreground"
        title="macOS 暂无法解密图片：微信 4.x 的图片密钥由自研加密处理，不经过系统加密接口"
      >
        <ImageIcon className="h-3.5 w-3.5" />
        <span className="text-xs">[图片]</span>
      </div>
    )
  }

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
        <div className="flex flex-col items-center justify-center w-32 h-32 bg-muted text-muted-foreground p-2 rounded-lg">
          <ImageIcon className="w-8 h-8 mb-1" />
          <span className="text-[10px]">加载失败</span>
        </div>
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
