import type { QueryClient } from '@tanstack/react-query'
import type { SyncStatus } from '@/lib/types'

/** 某账户同步状态的缓存键。只有这一个键，所有写入方与读者都用它。 */
export function syncStatusKey(accountId: number): readonly unknown[] {
  return ['sync-status', accountId]
}

/**
 * 这份状态是不是比手上那份**更新**。
 *
 * 判据是后端每次变更都会刷新的 `updated_at`。缺失时一律当作"更新"——
 * 老后端不带这个字段，那时退回"后写的赢"，与加这层守卫之前的行为一致。
 */
export function isNewerStatus(prev: SyncStatus | undefined, next: SyncStatus): boolean {
  if (prev?.updated_at == null || next.updated_at == null) return true
  return next.updated_at >= prev.updated_at
}

/**
 * 把一份同步状态写进缓存——**只在它更新时**。
 *
 * ── 为什么必须单调 ─────────────────────────────────────────────────────────
 *
 * 这个键有三个写入方：手动触发时的乐观播种、1 秒一次的轮询、SSE 推送。
 * 它们的到达顺序与产生顺序**不一致**：一个在同步期间发出、尚未落地的轮询请求，
 * 可能在 SSE 推来 `done` 之后才返回，带着旧快照 `{phase:'messages'}`。
 *
 * 不守的话有两种表现，后一种是致命的：
 *
 * 1. 同步进行中：轮询响应总比最新的 SSE 快照旧 → **进度条往回跳一格**。
 * 2. 同步结束后：`done` 让手动那一路收手（`enabled:false`、不再轮询），
 *    此时残留的轮询响应把状态改回 `messages`——而**再没有任何东西会来纠正它**。
 *    侧栏那个账户于是一直转圈、进度条停在「第 5 / 12 个文件夹」，
 *    直到该账户下一轮后台同步（默认 3 分钟）才恢复；账户被停用或删除则永久残留。
 *
 * 第 2 条是「每个账户行直接读裸缓存」之后才暴露的：在那之前 Shell 用
 * `syncingAccountId` 给状态开了道门，残留写入无人读得到。
 */
export function writeSyncStatus(qc: QueryClient, accountId: number, next: SyncStatus): void {
  qc.setQueryData<SyncStatus>(syncStatusKey(accountId), (prev) =>
    isNewerStatus(prev, next) ? next : prev,
  )
}
