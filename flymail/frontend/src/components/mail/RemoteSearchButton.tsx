// 「在服务器上搜索」兜底入口。
//
// 本地库只有已同步下来的邮件，同步深度之外的历史信搜不到——这个按钮把同一查询
// 翻译成 IMAP SEARCH 发给所有启用账户，把服务器命中而本地没有的邮件补抓入库。
//
// 补抓完不展示单独的「服务端结果」列表：mutation 成功后失效 ['messages'] 前缀，
// 本地搜索自己重跑一遍就把新邮件带出来了，用户看到的仍是同一个列表。
//
// 自带 mutation 与 toast，不向上暴露加载态——列表底部和空态两处都要放这个按钮，
// 状态提到 MailList 里只会让那个本就臃肿的组件再多三个变量。

import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { useToast } from '@/components/ui/Toast'
import { useRemoteSearch } from '@/lib/queries'

interface Props {
  /** 当前搜索串，原样发给后端 */
  q: string
}

export function RemoteSearchButton({ q }: Props) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const remote = useRemoteSearch()

  function run() {
    const query = q.trim()
    if (!query) return
    remote.mutate(query, {
      onSuccess: (res) => {
        // toast 只有一档样式，失败账户数只能并进同一句话里说
        const base = res.fetched > 0
          ? t('list.remoteSearch.added', { count: res.fetched })
          : t('list.remoteSearch.none')
        const failed = res.errors?.length ?? 0
        toast(failed > 0 ? `${base} · ${t('list.remoteSearch.errors', { count: failed })}` : base)
      },
      onError: () => toast(t('list.remoteSearch.failed')),
    })
  }

  return (
    <button
      type="button"
      className="pill-btn remote-search-btn"
      onClick={run}
      disabled={remote.isPending || q.trim().length === 0}
    >
      <Icon name="cloud" size={13} />
      <span>
        {remote.isPending ? t('list.remoteSearch.running') : t('list.remoteSearch.action')}
      </span>
    </button>
  )
}
