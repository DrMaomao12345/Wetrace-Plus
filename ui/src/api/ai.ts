import { request } from "@/lib/request";

export interface AISummarizeRequest {
  talker: string;
  time_range?: string;
  custom_prompt?: string;
  retry_of?: string;
}

export interface SummaryHistoryItem {
  id: string;
  talker: string;
  time_range: string;
  prompt_used: string;
  summary: string;
  msg_count: number;
  status: "success" | "failed" | "cancelled" | "running";
  error: string;
  retry_count: number;
  retry_of: string;
  created_at: string;
}

export interface AISimulateRequest {
  talker: string;
  message: string;
  conversation?: AISimulateTurn[];
  response_mode?: 'text' | 'voice';
}

export interface AISimulateTurn {
  role: 'user' | 'assistant';
  content: string;
}

export type ContactMemoryStatus = 'missing' | 'generating' | 'ready' | 'stale' | 'failed';

export interface ContactMemoryCatchphrase {
  text: string;
  evidence_seqs?: number[];
}

export interface ContactMemorySpeakingStyle {
  tone?: string[];
  sentence_length?: string;
  vocabulary?: string[];
  catchphrases?: ContactMemoryCatchphrase[];
  emoji_habits?: string[];
  punctuation_habits?: string[];
  response_patterns?: string[];
}

export interface ContactMemoryFact {
  content: string;
  subject?: string;
  stability?: string;
  observed_at?: string;
  evidence_seqs?: number[];
  confidence: string;
}

export interface ContactMemoryInteractionPattern {
  context: string;
  response: string;
  evidence_seqs?: number[];
}

export interface ContactMemoryTypicalExample {
  user: string;
  contact: string;
  evidence_seqs?: number[];
}

export interface ContactMemoryProfile {
  summary?: string;
  traits?: string[];
  speaking_style: ContactMemorySpeakingStyle;
  facts?: ContactMemoryFact[];
  interaction_patterns?: ContactMemoryInteractionPattern[];
  typical_examples?: ContactMemoryTypicalExample[];
}

export interface ContactMemorySource {
  min_seq?: number;
  max_seq?: number;
  last_message_at?: string;
  message_count: number;
  data_version?: string;
  voice_transcript_count?: number;
  voice_transcript_ids?: string[];
  voice_transcript_hash?: string;
  content_hash?: string;
}

export interface ContactMemoryGenerator {
  provider?: string;
  model?: string;
  prompt_version: string;
  generated_at: string;
}

export interface ContactMemoryUserOverrides {
  notes?: string[];
  style_instructions?: string[];
  pinned_facts?: string[];
  excluded_facts?: string[];
  updated_at?: string;
}

export interface ContactMemory {
  schema_version: number;
  account_id: string;
  talker: string;
  target_name?: string;
  status?: ContactMemoryStatus;
  profile: ContactMemoryProfile;
  source: ContactMemorySource;
  generator: ContactMemoryGenerator;
  user_overrides?: ContactMemoryUserOverrides;
  created_at: string;
  updated_at: string;
}

export interface ContactMemoryStatusResponse {
  exists: boolean;
  status: ContactMemoryStatus;
  memory?: ContactMemory;
  error?: string;
}

export interface DeleteContactMemoryResponse {
  deleted: boolean;
}

export interface AITodosRequest {
  talker: string;
  time_range?: string;
}

export interface TodoItem {
  content: string;
  deadline: string;
  priority: string;
  source_msg: string;
  source_time: string;
}

export interface AITodosResponse {
  todos: TodoItem[];
}

export interface AIExtractRequest {
  talker: string;
  time_range?: string;
  types?: string[];
}

export interface ExtractionItem {
  type: string;
  value: string;
  context: string;
  time: string;
}

export interface AIExtractResponse {
  extractions: ExtractionItem[];
}

export const aiApi = {
  summarize: (data: AISummarizeRequest) =>
    request.post<string>('/api/v1/ai/summarize', data),
  cancelSummarize: () =>
    request.post('/api/v1/ai/summarize/cancel', {}),
  getSummaryHistory: () =>
    request.get<SummaryHistoryItem[]>('/api/v1/ai/summary_history'),
  deleteSummaryHistory: (id: string) =>
    request.delete(`/api/v1/ai/summary_history/${id}`),
  simulate: (data: AISimulateRequest, signal?: AbortSignal) =>
    request.post<string>('/api/v1/ai/simulate', data, { signal }),
  getContactMemory: (talker: string, signal?: AbortSignal) =>
    request.get<ContactMemoryStatusResponse>(
      `/api/v1/ai/memories/${encodeURIComponent(talker)}`,
      undefined,
      { signal },
    ),
  rebuildContactMemory: (talker: string, signal?: AbortSignal) =>
    request.post<ContactMemoryStatusResponse>(
      `/api/v1/ai/memories/${encodeURIComponent(talker)}/rebuild`,
      {},
      { signal },
    ),
  deleteContactMemory: (talker: string, signal?: AbortSignal) =>
    request.delete<DeleteContactMemoryResponse>(
      `/api/v1/ai/memories/${encodeURIComponent(talker)}`,
      undefined,
      { signal },
    ),
  extractTodos: (data: AITodosRequest) =>
    request.post<AITodosResponse>('/api/v1/ai/todos', data),
  extractInfo: (data: AIExtractRequest) =>
    request.post<AIExtractResponse>('/api/v1/ai/extract', data),
};
