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
     * 加密图片是否解不出来。注意这不等于「所有图片都看不了」：
     * 2025 年 5 月之前的图片以明文 `_M.dat` 存放，可以正常显示；
     * 之后微信改为加密存储，那部分才需要图片密钥（目前提取不到）。
     * 所以这个标记只用来解释单张图片为什么失败，不要拿它去整体禁用图片功能。
     */
    imagesUnavailable: platform === "darwin" && !hasImageKey,
  }
}
