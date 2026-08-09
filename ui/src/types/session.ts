/**
 * 会话接口
 */
export interface Session {
  id: string
  talker: string
  talkerName: string
  name?: string
  avatar: string
  smallHeadURL?: string
  remark?: string
  type?: 'private' | 'group' | 'official' | 'unknown'
  lastMessage?: {
    nickName: string
    content: string
    createTime: number
    type: number
  }
  lastTime: string
  lastMessageType: number
  unreadCount: number
  isPinned: boolean
  isLocalPinned?: boolean
  isMinimized: boolean
  isChatRoom: boolean
  messageCount: number
  /** 会话类型标签（后端分类，手动覆盖优先） */
  tag?: TalkerTag
}

/** 会话类型标签，与后端 model.TalkerType 一一对应 */
export type TalkerTag =
  | 'friend'
  | 'group'
  | 'subscription'
  | 'service'
  | 'work'
  | 'stranger'
  | 'system'
  | 'other'

export const TALKER_TAG_LABELS: Record<TalkerTag, string> = {
  friend: '联系人',
  group: '群聊',
  subscription: '订阅号',
  service: '服务号',
  work: '企业微信',
  stranger: '陌生人',
  system: '系统账号',
  other: '其他',
}