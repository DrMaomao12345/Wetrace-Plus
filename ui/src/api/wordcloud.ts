import { request } from "@/lib/request";

export interface WordItem {
  text: string;
  count: number;
}

export interface WordCloudResponse {
  total_messages: number;
  total_words: number;
  words: WordItem[];
}

export interface WordCloudParams {
  time_range?: string;
  sender?: string;
  limit?: number;
  chunks?: number;
  min_chunks?: number;
}

export const wordcloudApi = {
  getWordCloud: (id: string, params?: WordCloudParams) =>
    request.get<WordCloudResponse>(`/api/v1/analysis/wordcloud/${id}`, params),

  getGlobalWordCloud: (params?: WordCloudParams) =>
    request.get<WordCloudResponse>("/api/v1/analysis/wordcloud/global", params),

  reloadStopwords: () =>
    request.post("/api/v1/analysis/wordcloud/reload_stopwords"),

  getDict: (type: "dict" | "stopwords") =>
    request.get<{ type: string; file: string; words: string[] }>(
      "/api/v1/analysis/wordcloud/dict", { type }
    ),

  updateDict: (type: "dict" | "stopwords", body: { add?: string[]; remove?: string[]; words?: string[] }) =>
    request.post<{ type: string; words: string[]; note: string }>(
      "/api/v1/analysis/wordcloud/dict",
      { type, ...body }
    ),
};
