import { useEffect, useState } from 'react'
import { useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { RefreshCw, X } from 'lucide-react'
import api from '@/services/api'
import { useAuthStore } from '@/stores/auth'
import { Button } from '@/components/ui/button'

const APP_NAME = 'Mail2IM'

type EmailData = {
  id?: number
  subject?: string
  from?: string
  to?: string
  mailbox?: string
  mailbox_path?: string
  received_at?: string
  html_body?: string
  text_body?: string
}

function formatDate(dateStr: string) {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  return isNaN(d.getTime()) ? '-' : d.toLocaleString()
}

function StandaloneHeader({
  email,
  loading,
  onRefresh,
  onClose,
  showClose,
}: {
  email: EmailData | null
  loading: boolean
  onRefresh: () => void
  onClose?: () => void
  showClose: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-between gap-4 flex-wrap px-5 py-3 border-b bg-background">
      <div className="flex flex-col gap-0.5 min-w-0">
        <h1 className="text-lg font-semibold text-foreground truncate">
          {email?.subject || '-'}
        </h1>
        <div className="flex gap-2 text-sm text-muted-foreground flex-wrap">
          {email?.mailbox && (
            <span>{t('emails_view.mailbox')}: {email.mailbox || email.mailbox_path || '-'}</span>
          )}
          {email?.from && (
            <>
              <span>·</span>
              <span>{t('common.from')}: {email.from}</span>
            </>
          )}
          {email?.to && (
            <>
              <span>·</span>
              <span>{t('common.to')}: {email.to}</span>
            </>
          )}
          {email?.received_at && (
            <>
              <span>·</span>
              <span>{t('common.received_at')}: {formatDate(email.received_at)}</span>
            </>
          )}
        </div>
      </div>
      <div className="flex items-center gap-2 shrink-0">
        <Button
          variant="ghost"
          size="sm"
          onClick={onRefresh}
          disabled={loading}
          title={t('common.refresh')}
        >
          <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
        </Button>
        {showClose && (
          <Button
            variant="ghost"
            size="sm"
            onClick={onClose}
            title={t('common.close')}
          >
            <X className="h-4 w-4" />
          </Button>
        )}
      </div>
    </div>
  )
}

export function EmailStandalonePage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const accessToken = useAuthStore((s) => s.accessToken)

  const [email, setEmail] = useState<EmailData | null>(null)
  const [loading, setLoading] = useState(false)
  const [htmlSrc, setHtmlSrc] = useState('')

  const fetchEmail = async () => {
    if (!id) return
    setLoading(true)
    try {
      const res = await api.get(`/emails/${id}`)
      setEmail(res.data as EmailData)
    } catch {
      toast.error(t('emails_view.load_error'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchEmail()
  }, [id])

  useEffect(() => {
    if (!id) return
    const tokenQuery = accessToken ? `?access_token=${encodeURIComponent(accessToken)}` : ''
    setHtmlSrc(`/api/emails/${id}/html${tokenQuery}`)
  }, [id, accessToken])

  useEffect(() => {
    document.title = email?.subject ? `${email.subject} - ${APP_NAME}` : APP_NAME
    return () => { document.title = APP_NAME }
  }, [email?.subject])

  const handleClose = () => window.close()

  return (
    <div className="flex flex-col min-h-screen bg-background">
      <StandaloneHeader
        email={email}
        loading={loading}
        onRefresh={fetchEmail}
        onClose={handleClose}
        showClose
      />
      <div className="flex-1 border-t overflow-hidden" style={{ minHeight: '0' }}>
        {loading ? (
          <div className="flex items-center justify-center h-full min-h-[60vh] text-muted-foreground">
            <RefreshCw className="h-6 w-6 animate-spin" />
          </div>
        ) : htmlSrc ? (
          <iframe
            src={htmlSrc}
            className="w-full border-none bg-white"
            style={{ height: 'calc(100vh - 80px)' }}
            sandbox="allow-same-origin allow-popups"
            referrerPolicy="no-referrer"
            title={email?.subject || 'Email'}
          />
        ) : (
          <div className="flex items-center justify-center h-full min-h-[60vh] text-muted-foreground text-sm">
            {t('emails_view.load_error')}
          </div>
        )}
      </div>
    </div>
  )
}

export function SharePage() {
  const { t } = useTranslation()
  const { token } = useParams<{ token: string }>()

  const [email, setEmail] = useState<EmailData | null>(null)
  const [loading, setLoading] = useState(false)
  const [srcDoc, setSrcDoc] = useState('')

  const fetchEmail = async () => {
    if (!token) return
    setLoading(true)
    try {
      const res = await api.get(`/public/emails/${token}`)
      const data = res.data as EmailData
      setEmail(data)
      if (data.html_body) {
        setSrcDoc(data.html_body)
      } else if (data.text_body) {
        setSrcDoc(`<pre style="white-space: pre-wrap; font-family: sans-serif;">${data.text_body}</pre>`)
      } else {
        setSrcDoc('')
      }
    } catch {
      toast.error(t('emails_view.load_error'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchEmail()
  }, [token])

  useEffect(() => {
    document.title = email?.subject ? `${email.subject} - ${APP_NAME}` : APP_NAME
    return () => { document.title = APP_NAME }
  }, [email?.subject])

  return (
    <div className="flex flex-col min-h-screen bg-background">
      <StandaloneHeader
        email={email}
        loading={loading}
        onRefresh={fetchEmail}
        showClose={false}
      />
      <div className="flex-1 overflow-hidden">
        {loading ? (
          <div className="flex items-center justify-center h-full min-h-[60vh] text-muted-foreground">
            <RefreshCw className="h-6 w-6 animate-spin" />
          </div>
        ) : srcDoc ? (
          <iframe
            srcDoc={srcDoc}
            className="w-full border-none bg-white"
            style={{ height: 'calc(100vh - 80px)' }}
            sandbox="allow-same-origin allow-popups"
            referrerPolicy="no-referrer"
            title={email?.subject || 'Email'}
          />
        ) : (
          <div className="flex items-center justify-center h-full min-h-[60vh] text-muted-foreground text-sm">
            {t('emails_view.load_error')}
          </div>
        )}
      </div>
    </div>
  )
}
