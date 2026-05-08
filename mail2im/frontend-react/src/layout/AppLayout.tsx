import { Outlet, NavLink, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth'
import { Sheet, SheetContent, SheetTrigger } from '@/components/ui/sheet'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  LayoutDashboard, Mail, Users, ScrollText, Settings, Bug,
  Globe, Bell, Tags, FileText, Shield, Menu,
  LogOut, User, Sun,
} from 'lucide-react'
import { useState, useRef, useEffect } from 'react'
import { ProfileDialog } from '@/components/ProfileDialog'
import { ThemePicker, initThemeFromStorage } from '@/components/ThemePicker'
import { Button } from '@/components/ui/button'

// ─── 初始化主题（模块级，首次加载时执行）────────────────────────────────────
initThemeFromStorage()

// ─── 菜单数据 ─────────────────────────────────────────────────────────────────

const menuGroups = [
  {
    label: 'menu.workspace',
    items: [
      { to: '/', icon: LayoutDashboard, label: 'nav.dashboard' },
      { to: '/accounts', icon: Users, label: 'nav.accounts' },
      { to: '/emails', icon: Mail, label: 'nav.emails' },
      { to: '/logs', icon: ScrollText, label: 'nav.logs' },
    ],
  },
  {
    label: 'menu.configuration',
    items: [
      { to: '/channels', icon: Bell, label: 'nav.channels' },
      { to: '/proxies', icon: Globe, label: 'nav.proxies' },
      { to: '/classification', icon: Tags, label: 'nav.classification' },
      { to: '/templates', icon: FileText, label: 'nav.templates' },
      { to: '/notification-policy', icon: Shield, label: 'nav.notification_policy' },
    ],
  },
  {
    label: 'menu.tools',
    items: [
      { to: '/settings', icon: Settings, label: 'nav.settings' },
      { to: '/dev', icon: Bug, label: 'nav.debug' },
    ],
  },
]

// ─── NavItem ──────────────────────────────────────────────────────────────────

function NavItem({
  to, icon: Icon, label, onClick,
}: {
  to: string
  icon: React.ElementType
  label: string
  onClick?: () => void
}) {
  const { t } = useTranslation()
  return (
    <NavLink
      to={to}
      end={to === '/'}
      onClick={onClick}
      className={({ isActive }) => isActive ? 'mm-side-btn mm-side-btn--active' : 'mm-side-btn'}
    >
      <Icon size={14} style={{ flexShrink: 0 }} />
      {t(label)}
    </NavLink>
  )
}

// ─── ThemePopover ─────────────────────────────────────────────────────────────

function ThemePopover() {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  // 点击外部关闭
  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <button
        title="主题"
        onClick={() => setOpen((v) => !v)}
        style={{
          width: 28, height: 28, borderRadius: 6,
          display: 'grid', placeItems: 'center',
          color: 'var(--ink-3)',
          background: 'none', border: 'none', cursor: 'pointer',
          transition: 'background 0.1s',
        }}
        onMouseEnter={(e) => { (e.currentTarget as HTMLButtonElement).style.background = 'var(--bg-hover)'; (e.currentTarget as HTMLButtonElement).style.color = 'var(--ink)' }}
        onMouseLeave={(e) => { (e.currentTarget as HTMLButtonElement).style.background = 'none'; (e.currentTarget as HTMLButtonElement).style.color = 'var(--ink-3)' }}
      >
        <Sun size={14} />
      </button>

      {open && (
        <div style={{
          position: 'fixed',
          bottom: 56,
          left: 8,
          zIndex: 70,
          width: 280,
          background: 'var(--surface)',
          border: '1px solid var(--rule)',
          borderRadius: 12,
          boxShadow: '0 20px 50px rgba(0,0,0,0.18), 0 0 0 1px rgba(0,0,0,0.06)',
          overflow: 'hidden',
        }}>
          {/* 标题栏 */}
          <div style={{
            padding: '12px 14px',
            borderBottom: '1px solid var(--rule)',
            display: 'flex', alignItems: 'center',
          }}>
            <span style={{
              fontFamily: 'var(--font-display, Georgia, serif)',
              fontSize: 15, color: 'var(--ink)',
              fontWeight: 500, letterSpacing: '-0.01em',
            }}>
              外观主题
            </span>
          </div>
          <ThemePicker onClose={() => setOpen(false)} />
        </div>
      )}
    </div>
  )
}

// ─── SidebarContent ───────────────────────────────────────────────────────────

function SidebarContent({ onNavClick }: { onNavClick?: () => void }) {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const clear = useAuthStore((s) => s.clear)
  const navigate = useNavigate()
  const [profileOpen, setProfileOpen] = useState(false)

  const handleLogout = () => {
    clear()
    navigate('/login')
  }

  return (
    <div style={{
      display: 'flex', flexDirection: 'column', height: '100%',
      background: 'var(--bg-alt)',
      fontFamily: 'var(--font-body, -apple-system, sans-serif)',
    }}>
      {/* Brand 区域 */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10,
        padding: '18px 16px 14px', height: 56,
        borderBottom: '1px solid var(--rule)',
        flexShrink: 0,
      }}>
        <div style={{
          width: 24, height: 24, borderRadius: 6,
          background: 'var(--mm-accent)',
          display: 'grid', placeItems: 'center',
          color: 'white',
          fontFamily: 'var(--font-display, Georgia, serif)',
          fontSize: 14, fontWeight: 600,
          flexShrink: 0,
        }}>
          M
        </div>
        <div style={{
          fontFamily: 'var(--font-display, Georgia, serif)',
          fontSize: 16, fontWeight: 500,
          letterSpacing: '-0.01em',
          color: 'var(--ink)',
          flex: 1,
        }}>
          Mail2IM
        </div>
      </div>

      {/* 导航滚动区 */}
      <div style={{ flex: 1, overflowY: 'auto', padding: '8px 8px 16px' }}>
        {menuGroups.map((group) => (
          <div key={group.label} style={{ marginBottom: 4 }}>
            {/* 分组标签 */}
            <div style={{
              padding: '14px 12px 6px',
              fontSize: 11,
              letterSpacing: '0.08em',
              textTransform: 'uppercase',
              color: 'var(--ink-3)',
              fontWeight: 500,
            }}>
              {t(group.label)}
            </div>
            {group.items.map((item) => (
              <NavItem key={item.to} {...item} onClick={onNavClick} />
            ))}
          </div>
        ))}
      </div>

      {/* 底部用户区 */}
      <div style={{
        borderTop: '1px solid var(--rule)',
        padding: 8,
        display: 'flex', alignItems: 'center', gap: 8,
        flexShrink: 0,
      }}>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button style={{
              flex: 1, display: 'flex', alignItems: 'center', gap: 10,
              padding: '6px 8px', borderRadius: 6,
              background: 'none', border: 'none', cursor: 'pointer',
              minWidth: 0, textAlign: 'left',
              transition: 'background 0.1s',
            }}
              onMouseEnter={(e) => { (e.currentTarget as HTMLButtonElement).style.background = 'var(--bg-hover)' }}
              onMouseLeave={(e) => { (e.currentTarget as HTMLButtonElement).style.background = 'none' }}
            >
              {/* 头像 */}
              <div style={{
                width: 28, height: 28, borderRadius: 6,
                background: 'var(--mm-accent)',
                display: 'grid', placeItems: 'center',
                color: 'white',
                fontFamily: 'var(--font-body, sans-serif)',
                fontSize: 11, fontWeight: 600,
                flexShrink: 0,
              }}>
                {(user?.username ?? 'U').slice(0, 2).toUpperCase()}
              </div>
              <div style={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
                <div style={{
                  fontSize: 13, color: 'var(--ink)',
                  whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
                }}>
                  {user?.username || 'User'}
                </div>
              </div>
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            <DropdownMenuItem onClick={() => setProfileOpen(true)}>
              <User className="mr-2 h-4 w-4" />
              {t('profile.title')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handleLogout}>
              <LogOut className="mr-2 h-4 w-4" />
              {t('user.logout')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        {/* 主题切换按钮 */}
        <ThemePopover />
      </div>

      <ProfileDialog open={profileOpen} onOpenChange={setProfileOpen} />
    </div>
  )
}

// ─── AppLayout ────────────────────────────────────────────────────────────────

export function AppLayout() {
  const [mobileOpen, setMobileOpen] = useState(false)

  return (
    <div className="flex h-screen overflow-hidden">
      {/* 桌面侧边栏 */}
      <aside className="hidden md:flex md:flex-col" style={{ width: 248, flexShrink: 0 }}>
        <SidebarContent />
      </aside>

      {/* 移动端顶栏 + Sheet */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <header
          className="flex md:hidden items-center h-14 px-4"
          style={{ borderBottom: '1px solid var(--rule)', background: 'var(--bg)' }}
        >
          <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
            <SheetTrigger asChild>
              <Button variant="ghost" size="icon">
                <Menu className="h-5 w-5" />
              </Button>
            </SheetTrigger>
            <SheetContent side="left" className="p-0" style={{ width: 248 }}>
              <SidebarContent onNavClick={() => setMobileOpen(false)} />
            </SheetContent>
          </Sheet>
          <span className="ml-3 font-semibold" style={{ color: 'var(--ink)' }}>Mail2IM</span>
        </header>

        {/* 主内容 */}
        <main className="flex-1 overflow-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
