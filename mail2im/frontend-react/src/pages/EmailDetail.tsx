import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ArrowLeft, ExternalLink, RefreshCw } from 'lucide-react'
import api from '@/services/api'
import { useAuthStore } from '@/stores/auth'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'

type EmailDetail = {
  id: number
  subject: string
  from: string
  to: string
  mailbox: string
  mailbox_path?: string
  mail_type: string
  received_at: string
  is_read: boolean
}

function formatDate(dateStr: string) {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  return isNaN(d.getTime()) ? '-' : d.toLocaleString()
}

function MetaItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex gap-2 items-baseline min-w-0">
      <span className="text-muted-foreground text-sm shrink-0">{label}</span>
      <span className="text-sm text-foreground truncate" title={value}>
        {value || '-'}
      </span>
    </div>
  )
}

export function EmailDetailPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const accessToken = useAuthStore((s) => s.accessToken)

  const [htmlSrc, setHtmlSrc] = useState('')

  const {
    data: email,
    isLoading,
    isError,
    refetch,
  } = useQuery<EmailDetail>({
    queryKey: ['email', id],
    queryFn: async () => {
      const res = await api.get(`/emails/${id}`)
      return res.data as EmailDetail
    },
    enabled: !!id,
    retry: false,
  })

  useEffect(() => {
    if (!isError) return
    toast.error(t('emails_view.load_error'))
  }, [isError, t])

  useEffect(() => {
    if (!id) return
    const tokenQuery = accessToken ? `?access_token=${encodeURIComponent(accessToken)}` : ''
    setHtmlSrc(`/api/emails/${id}/html${tokenQuery}`)
  }, [id, accessToken])

  useEffect(() => {
    if (email?.subject) {
      document.title = `${email.subject} - Mail2IM`
    } else {
      document.title = 'Mail2IM'
    }
    return () => {
      document.title = 'Mail2IM'
    }
  }, [email?.subject])

  const goBack = () => {
    if (window.history.length > 1) {
      navigate(-1)
    } else {
      navigate('/emails')
    }
  }

  const openStandalone = () => {
    if (!id) return
    const tokenQuery = accessToken ? `?access_token=${encodeURIComponent(accessToken)}` : ''
    window.open(`/api/emails/${id}/html${tokenQuery}`, '_blank')
  }

  return (
    <div className="flex flex-col h-full gap-4 p-6">
      {/* Header */}
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" onClick={goBack} className="gap-1.5">
            <ArrowLeft className="h-4 w-4" />
            {t('common.back')}
          </Button>
          <Separator orientation="vertical" className="h-5" />
          <h1 className="text-lg font-semibold tracking-tight truncate max-w-md">
            {isLoading ? t('common.email_detail') : (email?.subject || t('common.email_detail'))}
          </h1>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => refetch()}
            disabled={isLoading}
            title={t('common.refresh')}
          >
            <RefreshCw className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`} />
          </Button>
          <Button variant="outline" size="sm" onClick={openStandalone} className="gap-1.5">
            <ExternalLink className="h-4 w-4" />
            {t('common.view')}
          </Button>
        </div>
      </div>

      {isLoading ? (
        <div className="flex-1 flex items-center justify-center text-muted-foreground">
          <div className="flex flex-col items-center gap-3">
            <RefreshCw className="h-6 w-6 animate-spin" />
            <span className="text-sm">{t('dashboard.loading')}</span>
          </div>
        </div>
      ) : isError ? (
        <div className="flex-1 flex items-center justify-center text-muted-foreground text-sm">
          {t('emails_view.load_error')}
        </div>
      ) : email ? (
        <div className="flex flex-col flex-1 min-h-0 gap-4">
          {/* Metadata card */}
          <div className="rounded-lg border border-border bg-card px-5 py-4 space-y-2.5">
            <h2 className="text-xl font-semibold leading-tight">{email.subject || '-'}</h2>
            <Separator />
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-8 gap-y-1.5">
              <MetaItem label={t('common.from')} value={email.from} />
              <MetaItem label={t('common.to')} value={email.to} />
              <MetaItem
                label={t('emails_view.mailbox')}
                value={email.mailbox || email.mailbox_path || '-'}
              />
              <MetaItem label={t('common.received_at')} value={formatDate(email.received_at)} />
            </div>
          </div>

          {/* iframe email content */}
          <div className="flex-1 min-h-0 rounded-lg border border-border overflow-hidden bg-white">
            {htmlSrc ? (
              <iframe
                src={htmlSrc}
                className="w-full h-full border-none"
                style={{ minHeight: '60vh' }}
                sandbox="allow-same-origin allow-popups"
                referrerPolicy="no-referrer"
                title={email.subject || 'Email'}
              />
            ) : (
              <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
                {t('emails_view.load_error')}
              </div>
            )}
          </div>
        </div>
      ) : null}
    </div>
  )
}
