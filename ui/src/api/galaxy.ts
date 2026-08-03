import { request } from '@/lib/request'

export interface LifeStage { name: string; start: string; end: string; manual?: boolean }
export interface UserProfile { birth_year: number; birth_month: number; life_stages: LifeStage[] }

export interface GalaxyNode {
  contact_id: string
  display_name: string
  avatar: string
  relationship_type: string
  relationship_label: string
  main_life_stage: string
  life_stage_count: number
  relationship_depth: number
  current_temperature: number
  continuity_score: number
  reciprocity_score: number
  composite_intimacy: number
  confidence: number
  recent_activity: number
  status: string
  target_angle: number
  target_radius: number
  x: number
  y: number
  total_messages: number
  sent_messages: number
  recv_messages: number
  first_time: number
  last_time: number
  active_months: number
  evidence: string[]
  manual_important: boolean
  excluded: boolean
}

export interface GalaxyMonthSegment { month: string; strength: number; message_count: number }
export interface GalaxyContactTimeline {
  contact_id: string; display_name: string; avatar: string; relationship_type: string
  segments: GalaxyMonthSegment[]; first_month: string; last_month: string
}
export interface GalaxyGlobalHeat {
  month: string; heat_score: number; active_relationships: number; strong_relationships: number; message_count: number
}
export interface GalaxyTimeline {
  granularity: string; months: string[]
  contacts: GalaxyContactTimeline[]; global_heat: GalaxyGlobalHeat[]
}

export interface RelationshipGraph {
  generated_at: string
  version: number
  profile: UserProfile
  nodes: GalaxyNode[]
  total_count: number
  timeline?: GalaxyTimeline
}

export const galaxyApi = {
  getProfile: () => request.get<{ exists: boolean; profile?: UserProfile }>(`/api/v1/galaxy/profile`),
  updateProfile: (birth_year: number, birth_month: number, life_stages?: LifeStage[]) =>
    request.put<{ profile: UserProfile }>(`/api/v1/galaxy/profile`, { birth_year, birth_month, life_stages }),
  getGraph: () => request.get<RelationshipGraph>(`/api/v1/galaxy/graph`),
  rebuild: (top = 50) => request.post<RelationshipGraph>(`/api/v1/galaxy/rebuild?top=${top}`),
  patchContact: (id: string, patch: { relationship_type?: string; main_life_stage?: string; manual_important?: boolean; hidden?: boolean; reset?: boolean }) =>
    request.patch(`/api/v1/galaxy/contact/${encodeURIComponent(id)}`, patch),
}
