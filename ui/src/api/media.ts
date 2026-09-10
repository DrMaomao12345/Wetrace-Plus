import { request, getApiBaseUrl } from "@/lib/request"

export const mediaApi = {
  getImageUrl: (id: string, path?: string): string => {
    const baseURL = getApiBaseUrl()
    let url = `${baseURL}/api/v1/media/image/${encodeURIComponent(id)}`
    if (path) {
      url += `?path=${encodeURIComponent(path)}`
    }
    return url
  },

  getThumbnailUrl: (id: string, path?: string): string => {
    const baseURL = getApiBaseUrl()
    let url = `${baseURL}/api/v1/media/image/${encodeURIComponent(id)}?thumb=1`
    if (path) {
      url += `&path=${encodeURIComponent(path)}`
    }
    return url
  },

  getVideoUrl: (id: string): string => {
    const baseURL = getApiBaseUrl()
    return `${baseURL}/api/v1/media/video/${encodeURIComponent(id)}`
  },

  getVoiceUrl: (id: string): string => {
    const baseURL = getApiBaseUrl()
    return `${baseURL}/api/v1/media/voice/${encodeURIComponent(id)}`
  },

  getFileUrl: (id: string): string => {
    const baseURL = getApiBaseUrl()
    return `${baseURL}/api/v1/media/file/${encodeURIComponent(id)}`
  },

  getAvatarUrl: (avatarPath: string): string => {
    const baseURL = getApiBaseUrl()
    if (!avatarPath) return ''
    if (avatarPath.startsWith('http://') || avatarPath.startsWith('https://')) return avatarPath
    // Avatar logic might depend on how it's served. 
    // If it's a relative path to a static asset or another API:
    return `${baseURL}${avatarPath.startsWith('/') ? '' : '/'}${avatarPath}`
  },

  isMediaMessage: (type: number): boolean => {
    return [3, 34, 43, 47, 49].includes(type)
  },

  getImageList: (params?: {
    talker?: string;
    time_range?: string;
    limit?: number;
    offset?: number;
  }) => {
    return request.get<{
      total: number;
      items: ImageListItem[];
    }>('/api/v1/media/images', params)
  },

  transcribeVoice: (id: string) => {
    return request.post<{ text: string; cached: boolean }>('/api/v1/media/voice/transcribe', { id })
  },

  getVoiceTranscript: (id: string) => {
    return request.get<{ text: string | null; cached: boolean }>('/api/v1/media/voice/transcript', { id })
  },

  transcribeSession: (talker: string, retryMissing = false) => {
    return request.post<{
      message: string
      status?: string
      skipped?: number
      missing?: number
    }>('/api/v1/media/voice/transcribe-session', { talker, retry_missing: retryMissing })
  },

  transcribeSessionStatus: () => {
    return request.get<{
      running: boolean
      total: number
      done: number
      errors: number
      talker: string
      skipped?: number
      missing?: number
      canceled?: boolean
      current_talker?: string
      current_name?: string
      last_text?: string
    }>('/api/v1/media/voice/transcribe-session/status')
  },

  stopTranscribeSession: () => {
    return request.post('/api/v1/media/voice/transcribe-session/stop', {})
  },

  getExportVoicesUrl: (talker: string, name?: string): string => {
    const baseURL = getApiBaseUrl()
    let url = `${baseURL}/api/v1/export/voices?talker=${encodeURIComponent(talker)}`
    if (name) {
      url += `&name=${encodeURIComponent(name)}`
    }
    return url
  },

  exportVoices: (params: { talker: string; name?: string; ids: string[] }) => {
    return request.post<Blob>('/api/v1/export/voices', params, { responseType: 'blob' })
  },
}

export interface ImageListItem {
  key: string;
  talker: string;
  talkerName: string;
  time: string;
  thumbnailUrl: string;
  fullUrl: string;
  seq: number;
  /** 该图片是加密存储、当前解不出来（macOS 上 2025-05 之后的图片） */
  encrypted?: boolean;
}
