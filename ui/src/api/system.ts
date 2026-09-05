import { request, getApiBaseUrl } from "@/lib/request";

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
}

export interface AIPromptsResponse {
  prompts: Record<string, string>;
  defaults: Record<string, string>;
}

/** 一段「这段日期用这个时区」的配置 */
export interface TZSegmentConfig {
  start_date: string;  // YYYY-MM-DD
  end_date: string;    // YYYY-MM-DD
  tz_offset: number;   // 分钟，东正西负（UTC+8 → 480）
}
export interface TZConfigResponse {
  default_offset: number;
  has_key: boolean;
  segments: TZSegmentConfig[];
}

export const systemApi = {
  getStatus: () => request.get("/api/v1/system/status"),
  activate: (license: string) => request.post("/api/v1/system/activate", { license }),
  getCompliance: () => request.get<ComplianceStatus>("/api/v1/system/compliance"),
  agreeCompliance: (version: string) => request.post("/api/v1/system/compliance/agree", { version }),

  // AI Config
  getAIConfig: () => request.get<AIConfig>("/api/v1/system/ai_config"),
  updateAIConfig: (data: AIConfigUpdate) => request.post("/api/v1/system/ai_config", data),
  // 带上当前表单的配置，让后端测「屏幕上填的」而不是「上次保存的」
  testAIConfig: (data?: AIConfigUpdate) => request.post("/api/v1/ai/test", data),

  // AI Prompts
  getAIPrompts: () => request.get<AIPromptsResponse>("/api/v1/system/ai_prompts"),
  updateAIPrompts: (prompts: Record<string, string>) => request.post("/api/v1/system/ai_prompts", { prompts }),

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

  // 时区口径的**唯一来源**：默认时区 + 时间分段。所有按时区分桶的统计都读它，
  // 年度报告也不再自己存一份。
  getTZConfig: () => request.get<TZConfigResponse>("/api/v1/system/tz_config"),
  updateTZConfig: (defaultOffset: number, segments: TZSegmentConfig[]) =>
    request.post<{ status: string }>("/api/v1/system/tz_config", {
      default_offset: defaultOffset,
      segments,
    }),

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
