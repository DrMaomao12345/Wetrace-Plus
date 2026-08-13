import { request, getApiBaseUrl } from "@/lib/request";
import type { AxiosRequestConfig } from "axios";

export interface ComplianceStatus {
  agreed: boolean;
  agreed_at: string;
  version: string;
}

export interface MobilePairing {
  id: string;
  token: string;
  label: string;
  created_at: number;   // 秒
  last_seen_at: number; // 秒，0 = 从未
}

export interface AIConfig {
  enabled: boolean;
  provider: string;
  model: string;
  base_url: string;
  api_key_masked: string;
  provider_keys_masked: Record<string, string>;
}

export interface AIConfigUpdate {
  enabled: boolean;
  provider?: string;
  model?: string;
  base_url?: string;
  api_key?: string;
}

export interface SyncConfig {
  enabled: boolean;
  interval_minutes: number;
  last_sync_time: string;
  last_sync_status: string;
  is_syncing: boolean;
}

export interface SyncConfigUpdate {
  enabled: boolean;
  interval_minutes: number;
}

export interface PasswordStatus {
  enabled: boolean;
  is_locked: boolean;
}

export interface BackupConfig {
  enabled: boolean;
  interval_hours: number;
  backup_path: string;
  format: string;
  last_backup_time: string;
  last_backup_status: string;
}

export interface BackupConfigUpdate {
  enabled: boolean;
  interval_hours: number;
  backup_path: string;
  format?: string;
}

export interface BackupHistoryItem {
  id: string;
  time: string;
  status: string;
  file_path: string;
  file_size: number;
  sessions_count: number;
}

export interface TTSConfig {
  enabled: boolean;
  /** 每次数据同步后自动把新语音转成文字 */
  auto?: boolean;
  provider: string;
  base_url: string;
  api_key_masked: string;
  model: string;
  voice: string;
  speed: number;
  format: string;
}

/** 本机 Whisper 可执行文件 / 模型的扫描结果 */
export interface WhisperFound {
  path: string;
  name: string;
  size_mb?: number;
  source: string;
}

export interface WhisperScanResult {
  binaries: WhisperFound[] | null;
  models: WhisperFound[] | null;
  searched: string[];
  platform: string;
}

export interface TTSConfigUpdate {
  enabled: boolean;
  provider?: string;
  base_url?: string;
  api_key?: string;
  model?: string;
  voice?: string;
  speed?: number;
  format?: string;
  local_mode?: boolean;
  local_binary?: string;
  local_model?: string;
  auto?: boolean;
}

export interface AIPromptsResponse {
  prompts: Record<string, string>;
  defaults: Record<string, string>;
}

export const systemApi = {
  decrypt: () => request.post("/api/v1/system/decrypt"),
  getStatus: () => request.get("/api/v1/system/status"),
  getWeChatDbKey: (config?: AxiosRequestConfig) =>
    request.get("/api/v1/system/wxkey/db", {}, { timeout: 130000, ...config }),
  getWeChatImageKey: (config?: AxiosRequestConfig) =>
    request.get("/api/v1/system/wxkey/image", {}, { timeout: 130000, ...config }),
  activate: (license: string) => request.post("/api/v1/system/activate", { license }),
  detectWeChatPath: () => request.get("/api/v1/system/detect/wechat_path"),
  detectDbPath: () => request.get("/api/v1/system/detect/db_path"),
  selectPath: (type: 'file' | 'folder') => request.post("/api/v1/system/select_path", { type }),
  updateConfig: (data: Record<string, string>) => request.post("/api/v1/system/config", data),
  getCompliance: () => request.get<ComplianceStatus>("/api/v1/system/compliance"),
  agreeCompliance: (version: string) => request.post("/api/v1/system/compliance/agree", { version }),

  // AI Config
  getAIConfig: () => request.get<AIConfig>("/api/v1/system/ai_config"),
  updateAIConfig: (data: AIConfigUpdate) => request.post("/api/v1/system/ai_config", data),
  testAIConfig: () => request.post("/api/v1/ai/test"),

  // AI Prompts
  getAIPrompts: () => request.get<AIPromptsResponse>("/api/v1/system/ai_prompts"),
  updateAIPrompts: (prompts: Record<string, string>) => request.post("/api/v1/system/ai_prompts", { prompts }),

  // Sync Config
  getSyncConfig: () => request.get<SyncConfig>("/api/v1/system/sync_config"),
  updateSyncConfig: (data: SyncConfigUpdate) => request.post("/api/v1/system/sync_config", data),
  triggerSync: () => request.post("/api/v1/system/sync"),
  getSyncStatus: () => request.get<SyncConfig>("/api/v1/system/sync_status"),

  // Password
  getPasswordStatus: () => request.get<PasswordStatus>("/api/v1/system/password/status"),
  setPassword: (old_password: string, new_password: string) =>
    request.post("/api/v1/system/password/set", { old_password, new_password }),
  verifyPassword: (password: string) =>
    request.post("/api/v1/system/password/verify", { password }),
  disablePassword: (password: string) =>
    request.post("/api/v1/system/password/disable", { password }),

  // Backup Config
  getBackupConfig: () => request.get<BackupConfig>("/api/v1/system/backup_config"),
  updateBackupConfig: (data: BackupConfigUpdate) => request.post("/api/v1/system/backup_config", data),
  runBackup: (sessionIds?: string[]) =>
    request.post("/api/v1/system/backup/run", sessionIds?.length ? { session_ids: sessionIds } : {}),
  getBackupHistory: (limit = 20, offset = 0) =>
    request.get<BackupHistoryItem[]>("/api/v1/system/backup/history", { limit, offset }),

  // TTS Config
  getTTSConfig: () => request.get<TTSConfig>("/api/v1/system/tts_config"),
  scanWhisperLocal: () =>
    request.get<WhisperScanResult>("/api/v1/system/tts_local/scan"),
  updateTTSConfig: (data: TTSConfigUpdate) => request.post("/api/v1/system/tts_config", data),

  // Data directory
  getDataDir: () => request.get<{ path: string }>("/api/v1/system/data_dir"),
  openDataDir: () => request.post<{ path: string }>("/api/v1/system/data_dir/open"),

  // 有效聊天记录起始时间（全局，影响往年同期对比的下界）
  getEffectiveChatStart: () => request.get<{ year: number }>("/api/v1/system/effective_chat_start"),
  updateEffectiveChatStart: (year: number) => request.post<{ year: number }>("/api/v1/system/effective_chat_start", { year }),

  // 默认时区（全局，影响联系人侧分析查询）
  getDefaultTimezone: () => request.get<{ offset: number; has_key: boolean }>("/api/v1/system/default_timezone"),
  updateDefaultTimezone: (offset: number) => request.post<{ offset: number }>("/api/v1/system/default_timezone", { offset }),

  // 当前消息 DB 文件的指纹（path+size+mtime 的 md5），用于客户端缓存校验
  getDataVersion: () => request.get<{ version: string }>("/api/v1/system/data_version"),

  // 移动端配对（iOS App）—— 多设备记录 + 历史
  listMobilePairings: () =>
    request.get<{ pairings: MobilePairing[] }>("/api/v1/system/mobile/pairings"),
  createMobilePairing: (label: string) =>
    request.post<MobilePairing>("/api/v1/system/mobile/pairings", { label }),
  deleteMobilePairing: (id: string) =>
    request.delete<{ status: string }>(`/api/v1/system/mobile/pairings/${id}`),
  renameMobilePairing: (id: string, label: string) =>
    request.put<{ status: string }>(`/api/v1/system/mobile/pairings/${id}`, { label }),
  // 更新日志
  getChangelog: () => request.get<{ content: string }>("/api/v1/system/changelog"),

  testTTSConfigUrl: (text?: string, voice?: string, speed?: number): string => {
    const baseURL = getApiBaseUrl();
    const params = new URLSearchParams();
    if (text) params.set("text", text);
    if (voice) params.set("voice", voice);
    if (speed !== undefined) params.set("speed", String(speed));
    return `${baseURL}/api/v1/system/tts_config/test?${params.toString()}`;
  },
};