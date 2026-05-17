import { request, getApiBaseUrl } from "@/lib/request"
import { ContactType } from "@/types"
import type { Contact, ContactParams } from "@/types"

interface BackendContact {
  userName: string
  alias: string
  remark: string
  nickName: string
  isFriend: boolean
  smallHeadImgUrl?: string
  bigHeadImgUrl?: string
}

function getAvatarUrl(username?: string): string {
  if (!username) return ''
  return `/avatar/${username}`
}

function transformContact(backendContact: BackendContact): Contact {
  let type: ContactType
  const u = backendContact.userName
  if (u.endsWith('@chatroom')) {
    type = ContactType.Chatroom
  } else if (u.startsWith('gh_')) {
    type = ContactType.Official
  } else if (u.endsWith('@openim') || /^\d+@/.test(u)) {
    // @openim = 企业微信对外联系人；纯数字@xxx 也归为企业
    type = ContactType.Enterprise
  } else {
    type = ContactType.Friend
  }

  const avatar = getAvatarUrl(backendContact.userName)

  return {
    wxid: backendContact.userName,
    nickname: backendContact.nickName || backendContact.userName,
    remark: backendContact.remark || '',
    alias: backendContact.alias || '',
    avatar,
    type,
    isStarred: false,
    isPinned: false,
    isMinimized: false,
    bigHeadImgUrl: backendContact.bigHeadImgUrl || '',
    smallHeadImgUrl: backendContact.smallHeadImgUrl || '',
    headImgMd5: '',
  }
}

export interface NeedContactItem {
  userName: string
  nickName: string
  remark: string
  smallHeadURL: string
  bigHeadURL: string
  lastContactTime: number
  daysSinceContact: number
}

export const contactApi = {
  getContacts: async (params?: ContactParams): Promise<Contact[]> => {
    // 显式指定大 limit，覆盖全局拦截器默认的 200
    const merged = { limit: 100000, ...(params || {}) }
    const response = await request.get<BackendContact[]>('/api/v1/contacts', merged)

    if (Array.isArray(response)) {
      return response.map(transformContact)
    }

    return []
  },

  getContactDetail: async (wxid: string): Promise<Contact> => {
    // Note: Detail endpoint usually remains singular or appends ID to plural
    // Assuming /api/v1/contacts/:id based on REST conventions, but doc doesn't specify detail endpoint.
    // Keeping singular /api/v1/contact/:id as fallback or guessing /api/v1/contacts/:id?
    // The doc only shows list endpoints. Let's assume standard REST: /api/v1/contacts/:id
    const response = await request.get<BackendContact>(`/api/v1/contacts/${encodeURIComponent(wxid)}`)
    return transformContact(response)
  },

  searchContacts: async (keyword: string): Promise<Contact[]> => {
    return contactApi.getContacts({ keyword })
  },

  getChatroomMembers: async (chatroomId: string): Promise<Contact[]> => {
    const chatroom = await contactApi.getContactDetail(chatroomId)
    if (!chatroom.memberList) {
      return []
    }

    const memberPromises = chatroom.memberList.map(wxid =>
      contactApi.getContactDetail(wxid).catch(() => null)
    )
    const members = await Promise.all(memberPromises)

    return members.filter((m): m is Contact => m !== null)
  },

  getDisplayName: (contact: Contact): string => {
    return contact.remark || contact.nickname || contact.alias || contact.wxid
  },

  exportContacts: (format: 'csv' | 'xlsx' = 'csv', keyword?: string): string => {
    const baseURL = getApiBaseUrl()
    const params = new URLSearchParams({ format })
    if (keyword) params.set('keyword', keyword)
    return `${baseURL}/api/v1/contacts/export?${params.toString()}`
  },

  getNeedContactList: async (days: number = 7): Promise<NeedContactItem[]> => {
    const response = await request.get<NeedContactItem[]>('/api/v1/contacts/need-contact', { days })
    if (Array.isArray(response)) {
      return response
    }
    return []
  },
}

export { getAvatarUrl }
