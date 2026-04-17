import { api } from '../axios'
import type { BaseResponse } from '../types'
import type {
  NotifyChannel,
  CreateNotifyChannelRequest,
  UpdateNotifyChannelRequest,
  NotifyLog,
  EventDefinition,
  TestNotificationRequest
} from '../types'

interface NotifyChannelListResponse {
  channels: NotifyChannel[]
  total: number
}

interface NotifyLogListResponse {
  logs: NotifyLog[]
  total: number
  page: number
  page_size: number
}

interface EventDefinitionsResponse {
  definitions: EventDefinition[]
}

class NotifyService {
  // 获取通知渠道列表
  async getChannels(enabled?: boolean): Promise<BaseResponse<NotifyChannelListResponse>> {
    const params = enabled !== undefined ? { enabled } : {}
    return await api.get<NotifyChannelListResponse>('/notify/channels', { params })
  }

  // 获取单个通知渠道详情
  async getChannel(id: string): Promise<BaseResponse<NotifyChannel>> {
    return await api.get<NotifyChannel>(`/notify/channels/${id}`)
  }

  // 创建通知渠道
  async createChannel(data: CreateNotifyChannelRequest): Promise<BaseResponse<NotifyChannel>> {
    return await api.post<NotifyChannel>('/notify/channels', data)
  }

  // 更新通知渠道
  async updateChannel(id: string, data: UpdateNotifyChannelRequest): Promise<BaseResponse> {
    return await api.put(`/notify/channels/${id}`, data)
  }

  // 删除通知渠道
  async deleteChannel(id: string): Promise<BaseResponse> {
    return await api.delete(`/notify/channels/${id}`)
  }

  // 测试通知渠道
  async testChannel(id: string): Promise<BaseResponse> {
    return await api.post(`/notify/channels/${id}/test`)
  }

  // 发送测试通知到所有启用的渠道
  async sendTestNotification(data: TestNotificationRequest): Promise<BaseResponse> {
    return await api.post('/notify/test', data)
  }

  // 获取事件定义列表
  async getEventDefinitions(): Promise<BaseResponse<EventDefinitionsResponse>> {
    return await api.get<EventDefinitionsResponse>('/notify/events')
  }

  // 获取通知日志
  async getLogs(params?: {
    channel_id?: string
    event_type?: string
    status?: 'pending' | 'success' | 'failed'
    start_time?: string
    end_time?: string
    page?: number
    page_size?: number
  }): Promise<BaseResponse<NotifyLogListResponse>> {
    return await api.get<NotifyLogListResponse>('/notify/logs', { params })
  }

  // 批量更新通道排序
  async updateChannelOrder(channelIds: string[]): Promise<BaseResponse> {
    // 这个接口可能需要后端支持，暂时使用批量更新的方式
    // 可以通过给每个通道设置一个 order 字段来实现
    const updates = channelIds.map((id, index) => 
      this.updateChannel(id, { config: { order: index } })
    )
    await Promise.all(updates)
    return { code: 0, message: 'success', data: null }
  }
}

export const notifyService = new NotifyService()