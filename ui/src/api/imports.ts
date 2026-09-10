import { request } from "@/lib/request"

export interface ImportFormat {
  id: string
  name: string
  extensions: string[]
  description: string
  self_hint: boolean
}

export interface ImportWarning {
  file: string
  row?: number
  code: string
  message: string
}

export interface ImportSourceReport {
  file: string
  format: string
  messages: number
  imported: number
  duplicates: number
  warnings?: ImportWarning[]
}

export interface ImportResult {
  files: number
  conversations: number
  messages: number
  imported: number
  duplicates: number
  started_at: string
  completed_at: string
  sources: ImportSourceReport[]
  warnings?: ImportWarning[]
}

export interface ImportHistoryItem {
  id: number
  source_name: string
  source_hash: string
  formats: string
  messages: number
  imported: number
  duplicates: number
  warning_count: number
  imported_at: string
}

export const importApi = {
  getFormats: () => request.get<ImportFormat[]>("/api/v1/imports/formats"),
  getHistory: (limit = 20) => request.get<ImportHistoryItem[]>("/api/v1/imports/history", { limit }),
  upload: (
    files: File[],
    options: { selfId?: string; selfName?: string },
    onProgress?: (percent: number) => void,
  ) => {
    const data = new FormData()
    files.forEach((file) => data.append("files", file))
    if (options.selfId?.trim()) data.append("self_id", options.selfId.trim())
    if (options.selfName?.trim()) data.append("self_name", options.selfName.trim())
    return request.post<ImportResult>("/api/v1/imports", data, {
      headers: { "Content-Type": "multipart/form-data" },
      timeout: 30 * 60 * 1000,
      onUploadProgress: (event) => {
        if (event.total && onProgress) onProgress(Math.round((event.loaded / event.total) * 100))
      },
    })
  },
}
