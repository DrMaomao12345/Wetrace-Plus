import { useQuery } from "@tanstack/react-query"
import { systemApi } from "@/api"

interface SystemStatus {
  platform?: string
  config?: {
    has_image_key?: boolean
    has_wechat_db_key?: boolean
  }
}

/**
 * 平台与密钥状态。走 react-query 共享缓存，整个应用只发一次请求 ——
 * 消息气泡这种会渲染成百上千次的组件也能安全调用。
 */
export function usePlatform() {
  const { data } = useQuery({
    queryKey: ["system-status"],
    queryFn: () => systemApi.getStatus() as Promise<SystemStatus>,
    staleTime: Infinity,
    gcTime: Infinity,
    retry: false,
  })

  const platform = data?.platform ?? ""
  const hasImageKey = data?.config?.has_image_key ?? false

  return {
    platform,
    isMac: platform === "darwin",
    hasImageKey,
    /**
     * 图片是否根本解不出来。macOS 上微信 4.x 的图片密钥由自研加密处理、
     * 不经过系统加密接口，目前提取不到 —— 这种情况下不该让用户看到「加载失败」，
     * 那会让人以为是网络或缓存问题。
     */
    imagesUnavailable: platform === "darwin" && !hasImageKey,
  }
}
