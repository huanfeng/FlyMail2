import { Outlet, NavLink, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { Sheet, SheetContent, SheetTrigger } from '@/components/ui/sheet'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  LayoutDashboard, Mail, Users, ScrollText, Settings, Bug,
  Globe, Bell, Tags, FileText, Shield, Menu, Moon, Sun,
  LogOut, User,
} from 'lucide-react'
import { useState } from 'react'
import { ProfileDialog } from '@/components/ProfileDialog'

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

function NavItem({ to, icon: Icon, label, onClick }: { to: string; icon: React.ElementType; label: string; onClick?: () => void }) {
  const { t } = useTranslation()
  return (
    <NavLink
      to={to}
      end={to === '/'}
      onClick={onClick}
      className={({ isActive }) =>
        cn(
          'flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors',
          isActive
            ? 'bg-accent text-accent-foreground font-medium'
            : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
        )
      }
    >
      <Icon className="h-4 w-4 shrink-0" />
      {t(label)}
    </NavLink>
  )
}

function SidebarContent({ onNavClick }: { onNavClick?: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col h-full">
      <div className="px-4 py-5">
        <h1 className="text-lg font-semibold tracking-tight">Mail2IM</h1>
      </div>
      <Separator />
      <ScrollArea className="flex-1 px-3 py-3">
        {menuGroups.map((group) => (
          <div key={group.label} className="mb-4">
            <p className="px-3 mb-1 text-xs font-medium text-muted-foreground uppercase tracking-wider">
              {t(group.label)}
            </p>
            <div className="space-y-0.5">
              {group.items.map((item) => (
                <NavItem key={item.to} {...item} onClick={onNavClick} />
              ))}
            </div>
          </div>
        ))}
      </ScrollArea>
      <Separator />
      <UserMenu />
    </div>
  )
}

function UserMenu() {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const clear = useAuthStore((s) => s.clear)
  const navigate = useNavigate()
  const [dark, setDark] = useState(document.documentElement.classList.contains('dark'))
  const [profileOpen, setProfileOpen] = useState(false)

  const toggleDark = () => {
    document.documentElement.classList.toggle('dark')
    const isDark = document.documentElement.classList.contains('dark')
    setDark(isDark)
    localStorage.setItem('mail2im_ui_theme_dark', String(isDark))
  }

  const handleLogout = () => {
    clear()
    navigate('/login')
  }

  return (
    <div className="p-3">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" className="w-full justify-start gap-2 h-auto py-2">
            <div className="h-7 w-7 rounded-full bg-primary/10 flex items-center justify-center">
              <User className="h-4 w-4 text-primary" />
            </div>
            <span className="text-sm font-medium truncate">{user?.username || 'User'}</span>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-48">
          <DropdownMenuItem onClick={() => setProfileOpen(true)}>
            <User className="mr-2 h-4 w-4" />
            {t('profile.title')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={toggleDark}>
            {dark ? <Sun className="mr-2 h-4 w-4" /> : <Moon className="mr-2 h-4 w-4" />}
            {dark ? t('settings.light_mode') : t('settings.dark_mode')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={handleLogout}>
            <LogOut className="mr-2 h-4 w-4" />
            {t('user.logout')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <ProfileDialog open={profileOpen} onOpenChange={setProfileOpen} />
    </div>
  )
}

export function AppLayout() {
  const [mobileOpen, setMobileOpen] = useState(false)

  // Initialize dark mode from localStorage
  useState(() => {
    const isDark = localStorage.getItem('mail2im_ui_theme_dark') === 'true'
    if (isDark) document.documentElement.classList.add('dark')
  })

  return (
    <div className="flex h-screen overflow-hidden">
      {/* Desktop sidebar */}
      <aside className="hidden md:flex md:w-64 md:flex-col border-r bg-sidebar">
        <SidebarContent />
      </aside>

      {/* Mobile header + sheet */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex md:hidden items-center h-14 border-b px-4 bg-background">
          <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
            <SheetTrigger asChild>
              <Button variant="ghost" size="icon">
                <Menu className="h-5 w-5" />
              </Button>
            </SheetTrigger>
            <SheetContent side="left" className="w-64 p-0">
              <SidebarContent onNavClick={() => setMobileOpen(false)} />
            </SheetContent>
          </Sheet>
          <span className="ml-3 font-semibold">Mail2IM</span>
        </header>

        {/* Main content */}
        <main className="flex-1 overflow-auto p-4 md:p-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
