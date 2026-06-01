import type { ReactNode } from 'react'

interface AppLayoutProps {
  sidebar: ReactNode
  list: ReactNode
  reader: ReactNode
}

// 三栏布局：248px 侧栏 / 380px 列表 / flex-1 阅读区（参考 MailMaster 比例）。
export function AppLayout({ sidebar, list, reader }: AppLayoutProps) {
  return (
    <div className="flex h-screen w-screen overflow-hidden">
      <aside
        className="flex-shrink-0 overflow-y-auto"
        style={{ width: 'var(--sidebar-w)', background: 'var(--bg-alt)', borderRight: '1px solid var(--rule)' }}
      >
        {sidebar}
      </aside>
      <section
        className="flex flex-shrink-0 flex-col overflow-hidden"
        style={{ width: 'var(--list-w)', borderRight: '1px solid var(--rule)' }}
      >
        {list}
      </section>
      <main className="min-w-0 flex-1 overflow-y-auto">{reader}</main>
    </div>
  )
}
