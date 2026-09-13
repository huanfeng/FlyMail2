import { describe, it, expect } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { isNewerStatus, syncStatusKey, writeSyncStatus } from '@/lib/sync-cache'
import type { SyncStatus } from '@/lib/types'

const AT = (s: string) => `2026-09-13T10:00:${s}Z`

function st(phase: SyncStatus['phase'], sec: string, extra: Partial<SyncStatus> = {}): SyncStatus {
  return { account_id: 1, phase, updated_at: AT(sec), ...extra }
}

/**
 * 同步状态缓存的写入必须**单调**。
 *
 * 这个键有三个写入方（触发时的乐观播种、1 秒一次的轮询、SSE 推送），
 * 它们的到达顺序与产生顺序不一致——一个在同步期间发出、尚未落地的轮询请求，
 * 可能在 SSE 推来 done 之后才返回，带着旧快照。
 *
 * 不守的后果分两档，后一档是致命的：同步中是进度条往回跳，
 * 而同步**结束后**那次残留写入会把状态改回 messages，**再没有东西来纠正它**——
 * 轮询已经停了，账户行只读缓存不发请求。侧栏于是永久转圈。
 */
describe('writeSyncStatus 的单调性', () => {
  it('更新的覆盖旧的', () => {
    const qc = new QueryClient()
    writeSyncStatus(qc, 1, st('messages', '01'))
    writeSyncStatus(qc, 1, st('done', '02'))
    expect(qc.getQueryData<SyncStatus>(syncStatusKey(1))?.phase).toBe('done')
  })

  it('旧的**不能**覆盖新的——这是那条「永久转圈」的根因', () => {
    const qc = new QueryClient()
    writeSyncStatus(qc, 1, st('done', '05'))
    // 在途的轮询响应迟到了，带着同步期间的旧快照
    writeSyncStatus(qc, 1, st('messages', '03', { folders_done: 5, folders_total: 12 }))

    const now = qc.getQueryData<SyncStatus>(syncStatusKey(1))
    expect(now?.phase, '旧快照把已完成的同步改回了进行中').toBe('done')
    expect(now?.folders_done).toBeUndefined()
  })

  it('同一时刻的后写者赢（>= 而不是 >）', () => {
    // 后端在同一毫秒内推两次是可能的（enterFolder 紧接着 finishFolder），
    // 用严格大于会把第二条丢掉，进度就少走一格。
    const qc = new QueryClient()
    writeSyncStatus(qc, 1, st('messages', '04', { folders_done: 1 }))
    writeSyncStatus(qc, 1, st('messages', '04', { folders_done: 2 }))
    expect(qc.getQueryData<SyncStatus>(syncStatusKey(1))?.folders_done).toBe(2)
  })

  it('触发时的乐观播种（没有 updated_at）总能覆盖上一轮遗留的 done', () => {
    // 用户刚按下按钮，这一刻服务端确实是 queued——后端 Trigger 在返回 202
    // 之前就同步置了。若因为它没有 updated_at 而被挡下，第二次点同步
    // 界面上就什么都不会发生（这是第五轮修过的那个回归）。
    const qc = new QueryClient()
    writeSyncStatus(qc, 1, st('done', '09'))
    writeSyncStatus(qc, 1, { account_id: 1, phase: 'queued' })
    expect(qc.getQueryData<SyncStatus>(syncStatusKey(1))?.phase).toBe('queued')
  })

  it('按账户分键，互不影响', () => {
    const qc = new QueryClient()
    writeSyncStatus(qc, 1, st('messages', '01'))
    writeSyncStatus(qc, 2, st('done', '01'))
    expect(qc.getQueryData<SyncStatus>(syncStatusKey(1))?.phase).toBe('messages')
    expect(qc.getQueryData<SyncStatus>(syncStatusKey(2))?.phase).toBe('done')
  })
})

describe('isNewerStatus', () => {
  it('缺 updated_at 时退回「后写的赢」', () => {
    // 老后端不带这个字段。那时的行为必须与加这层守卫之前一致，
    // 否则升级前端而没升级后端会让进度整个不动。
    expect(isNewerStatus(undefined, { phase: 'done' })).toBe(true)
    expect(isNewerStatus({ phase: 'done' }, { phase: 'messages' })).toBe(true)
    expect(isNewerStatus({ phase: 'done', updated_at: AT('05') }, { phase: 'messages' })).toBe(true)
  })

  it('两边都有时按时间比', () => {
    expect(isNewerStatus(st('messages', '01'), st('done', '02'))).toBe(true)
    expect(isNewerStatus(st('done', '02'), st('messages', '01'))).toBe(false)
  })
})
