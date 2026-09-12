import { describe, it, expect } from 'vitest'
import { InfiniteQueryObserver, QueryClient } from '@tanstack/react-query'
import type { InfiniteData } from '@tanstack/react-query'

/**
 * react-query 状态位的语义契约。
 *
 * 这不是在测我们自己的代码，而是把一条**我们依赖的第三方行为**钉死：
 * `status` 是整个 query 的，翻页失败同样会把它置为 'error' 并填上 error，
 * 而已加载的页原样留在缓存里。
 *
 * 为什么值得一个测试文件：列表的「首屏错误态」一度直接用了裸 `error`，
 * 于是第 2 页一失败就整屏错误面板，把屏幕上那 50 封邮件全吃掉——
 * 而为它写的组件测试是绿的，因为那个测试构造了 `nextPageError: true` 且
 * `error` 为空的组合，这个组合在真实链路里根本不可达。
 * 判据改成 `isLoadingError` 之后，如果哪次升级把这些位的含义改了，
 * 我们要在这里立刻知道，而不是靠用户报「邮件突然没了」。
 */
describe('useInfiniteQuery 的错误状态位', () => {
  /** 建一个「第 1 页成功、第 pageParam===1 页失败」的观察者 */
  async function observeFailedSecondPage() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const observer = new InfiniteQueryObserver<string[], Error, InfiniteData<string[]>, readonly unknown[], number>(qc, {
      queryKey: ['semantics'],
      initialPageParam: 0,
      getNextPageParam: (_last, _all, lastParam) => lastParam + 1,
      queryFn: async ({ pageParam }) => {
        if (pageParam === 1) throw new Error('第二页炸了')
        return [`page${pageParam}`]
      },
    })
    const unsub = observer.subscribe(() => {})
    await new Promise((r) => setTimeout(r, 20))
    const first = observer.getCurrentResult()
    await observer.fetchNextPage().catch(() => {})
    await new Promise((r) => setTimeout(r, 20))
    const after = observer.getCurrentResult()
    unsub()
    qc.clear()
    return { first, after }
  }

  it('翻页失败会把整个 query 置为 error，且数据仍在', async () => {
    const { first, after } = await observeFailedSecondPage()

    expect(first.status).toBe('success')
    expect(first.data?.pages).toHaveLength(1)

    // 这两条就是「裸 error 不能当首屏错误用」的全部理由
    expect(after.isError).toBe(true)
    expect(after.error).not.toBeNull()
    // 已加载的页没有丢——所以此时掀掉整张列表是纯粹的信息损失
    expect(after.data?.pages).toHaveLength(1)
  })

  it('isLoadingError 能把「翻页失败」与「首屏失败」分开', async () => {
    const { after } = await observeFailedSecondPage()

    // 有数据 ⇒ 不是首屏错误。列表该继续显示内容，失败落在底部那一行
    expect(after.isLoadingError).toBe(false)
    expect(after.isFetchNextPageError).toBe(true)
    // isLoading 同样挡不住：status 已是 error，isPending 为 false
    expect(after.isLoading).toBe(false)
  })

  it('首屏就失败时 isLoadingError 为真', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const observer = new InfiniteQueryObserver<string[], Error, InfiniteData<string[]>, readonly unknown[], number>(qc, {
      queryKey: ['semantics-first'],
      initialPageParam: 0,
      getNextPageParam: () => undefined,
      queryFn: async () => {
        throw new Error('首屏就炸了')
      },
    })
    const unsub = observer.subscribe(() => {})
    await new Promise((r) => setTimeout(r, 20))
    const r = observer.getCurrentResult()
    unsub()
    qc.clear()

    expect(r.isLoadingError).toBe(true)
    expect(r.data).toBeUndefined()
  })
})
