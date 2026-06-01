import { useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { AppLayout } from '@/components/mail/AppLayout'
import { AccountSidebar } from '@/components/mail/AccountSidebar'
import { MailList } from '@/components/mail/MailList'
import { Reader } from '@/components/mail/Reader'
import { useAccounts, useFolders, useMessages, useSyncStatus, useTriggerSync } from '@/lib/queries'

export function ShellPage() {
  // 注意：GET /folders/:fid/messages 不绑定 account，依赖单管理员假设；
  // 未来支持多用户时需补 ownership 校验。
  const qc = useQueryClient()
  const [params, setParams] = useSearchParams()
  const accountId = params.get('account') ? Number(params.get('account')) : null
  const folderId = params.get('folder') ? Number(params.get('folder')) : null
  const messageId = params.get('message') ? Number(params.get('message')) : null

  const { data: accounts = [] } = useAccounts()
  const { data: folders = [] } = useFolders(accountId)
  const { data: messages = [], isLoading: messagesLoading } = useMessages(folderId)

  const [syncEnabled, setSyncEnabled] = useState(false)
  const { data: syncStatus } = useSyncStatus(accountId, syncEnabled)
  const triggerSync = useTriggerSync()
  const syncing = syncStatus?.phase === 'folders' || syncStatus?.phase === 'messages'

  function setParam(mut: (p: URLSearchParams) => void, replace = false) {
    const next = new URLSearchParams(params)
    mut(next)
    setParams(next, { replace })
  }

  useEffect(() => {
    if (accountId == null && accounts.length > 0) {
      setParam((p) => p.set('account', String(accounts[0].id)), true)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accountId, accounts])

  useEffect(() => {
    if (syncStatus?.phase === 'done' || syncStatus?.phase === 'error') {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setSyncEnabled(false)
    }
    // 同步抓完后刷新文件夹（未读数/新文件夹）和邮件列表缓存。
    if (syncStatus?.phase === 'done') {
      void qc.invalidateQueries({ queryKey: ['folders'] })
      void qc.invalidateQueries({ queryKey: ['messages'] })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [syncStatus?.phase])

  const activeFolder = useMemo(
    () => folders.find((f) => f.id === folderId) ?? null,
    [folders, folderId],
  )

  function selectAccount(id: number) {
    setParam((p) => {
      p.set('account', String(id))
      p.delete('folder')
      p.delete('message')
    })
  }

  function selectFolder(id: number) {
    setParam((p) => {
      p.set('folder', String(id))
      p.delete('message')
    })
  }

  function selectMessage(id: number) {
    setParam((p) => p.set('message', String(id)))
  }

  function onSync(id: number) {
    setSyncEnabled(true)
    triggerSync.mutate(id)
  }

  return (
    <AppLayout
      sidebar={
        <AccountSidebar
          accounts={accounts}
          folders={folders}
          activeAccountId={accountId}
          activeFolderId={folderId}
          syncing={syncing}
          onSelectAccount={selectAccount}
          onSelectFolder={selectFolder}
          onSync={onSync}
        />
      }
      list={
        <MailList
          folder={activeFolder}
          messages={messages}
          loading={messagesLoading}
          activeMessageId={messageId}
          onSelectMessage={selectMessage}
        />
      }
      reader={<Reader />}
    />
  )
}
