import { useNavigate } from 'react-router'
import { auth } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { LogOut, CheckCircle2 } from 'lucide-react'

export function ShellPage() {
  const navigate = useNavigate()

  const handleLogout = () => {
    auth.clear()
    navigate('/login')
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center space-y-2">
          <div className="mx-auto h-12 w-12 rounded-xl bg-primary/10 flex items-center justify-center">
            <CheckCircle2 className="h-6 w-6 text-primary" />
          </div>
          <CardTitle className="text-xl font-semibold">已登录</CardTitle>
          <p className="text-sm text-muted-foreground">
            欢迎使用 FlyMail。邮件界面将在后续里程碑中实现。
          </p>
        </CardHeader>
        <CardContent>
          <Button variant="outline" className="w-full" onClick={handleLogout}>
            <LogOut className="h-4 w-4" />
            退出登录
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
