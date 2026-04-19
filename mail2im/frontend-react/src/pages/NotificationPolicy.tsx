import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  RefreshCw,
  Inbox,
  Wallet,
  Star,
  Bell,
  HelpCircle,
  Megaphone,
  Users,
  Ban,
  Trash2,
  FileEdit,
  Send,
  Tag,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import api, { extractList } from '@/services/api'
import { PageHeader } from '@/components/PageHeader'

// ─── Types ───────────────────────────────────────────────────────────────────

interface PolicyChannel {
  id: number
  name: string
  type: string
  selected: boolean
}

interface PolicyItem {
  ID: number
  key: string
  name: string
  priority: number
  is_system: boolean
  action: string
  channel_ids: string
  channels: PolicyChannel[]
}

// ─── Icon map ─────────────────────────────────────────────────────────────────

const TYPE_ICONS: Record<string, React.ElementType> = {
  primary: Inbox,
  bill: Wallet,
  important: Star,
  notification: Bell,
  unknown: HelpCircle,
  promotion: Megaphone,
  social: Users,
  spam: Ban,
  trash: Trash2,
  draft: FileEdit,
  sent: Send,
}

function TypeIcon({ mailKey }: { mailKey: string }) {
  const Icon = TYPE_ICONS[mailKey] ?? Tag
  return <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export function NotificationPolicyPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [saving, setSaving] = useState<string | null>(null)

  // Fetch policy list (channels embedded per item)
  const { data: items = [], isFetching } = useQuery<PolicyItem[]>({
    queryKey: ['notification-policy'],
    queryFn: async () => {
      const res = await api.get<PolicyItem[]>('/notification-policy')
      return extractList(res.data)
    },
  })

  // Single-item update mutation
  const updateMutation = useMutation({
    mutationFn: async ({ key, payload }: { key: string; payload: { channel_ids: string; action: string; priority: number } }) => {
      await api.put(`/notification-policy/${key}`, payload)
    },
    onSuccess: (_data, { key }) => {
      toast.success(t('policy.update_success'))
      setSaving(null)
      queryClient.invalidateQueries({ queryKey: ['notification-policy'] })
      // optimistic: key used to track spinner
      void key
    },
    onError: () => {
      toast.error(t('policy.update_error'))
      setSaving(null)
    },
  })

  const updateItem = (item: PolicyItem) => {
    setSaving(item.key)
    const selectedIds = item.channels.filter((c) => c.selected).map((c) => c.id)
    updateMutation.mutate({
      key: item.key,
      payload: {
        channel_ids: JSON.stringify(selectedIds),
        action: item.action,
        priority: item.priority,
      },
    })
  }

  const handleToggleChannel = (item: PolicyItem, channelId: number) => {
    // Mutate local query cache optimistically
    queryClient.setQueryData<PolicyItem[]>(['notification-policy'], (old) =>
      (old ?? []).map((it) =>
        it.key === item.key
          ? {
              ...it,
              channels: it.channels.map((ch) =>
                ch.id === channelId ? { ...ch, selected: !ch.selected } : ch
              ),
            }
          : it
      )
    )
    // Build updated item for server call
    const updatedItem: PolicyItem = {
      ...item,
      channels: item.channels.map((ch) =>
        ch.id === channelId ? { ...ch, selected: !ch.selected } : ch
      ),
    }
    updateItem(updatedItem)
  }

  const handleActionChange = (item: PolicyItem, newAction: string) => {
    queryClient.setQueryData<PolicyItem[]>(['notification-policy'], (old) =>
      (old ?? []).map((it) => (it.key === item.key ? { ...it, action: newAction } : it))
    )
    updateItem({ ...item, action: newAction })
  }

  const priorityLabel = (p: number) => {
    if (p >= 20) return t('common.priority_high')
    if (p >= 10) return t('common.priority_normal')
    return t('common.priority_low')
  }

  const priorityVariant = (p: number): 'default' | 'secondary' | 'outline' => {
    if (p >= 20) return 'default'
    if (p >= 10) return 'secondary'
    return 'outline'
  }

  return (
    <div className="flex flex-col gap-6 max-w-5xl">
      <PageHeader
        title={t('policy.title')}
        subtitle={t('policy.subtitle')}
        actions={
          <Button
            variant="ghost"
            size="icon"
            onClick={() => queryClient.invalidateQueries({ queryKey: ['notification-policy'] })}
            disabled={isFetching}
            aria-label={t('common.refresh')}
          >
            <RefreshCw className={`h-4 w-4 ${isFetching ? 'animate-spin' : ''}`} />
          </Button>
        }
      />

      {/* Table */}
      <div className="overflow-x-auto">
        {isFetching && items.length === 0 ? (
          <div className="flex items-center justify-center p-12 text-muted-foreground">
            <RefreshCw className="h-5 w-5 animate-spin mr-2" />
            {t('dashboard.loading')}
          </div>
        ) : (
          <table className="w-full text-sm border-collapse">
            <thead>
              <tr className="border-b">
                <th className="text-left px-4 py-3 font-medium text-muted-foreground w-44">
                  {t('policy.mail_type')}
                </th>
                <th className="text-left px-4 py-3 font-medium text-muted-foreground w-24">
                  {t('policy.priority')}
                </th>
                <th className="text-left px-4 py-3 font-medium text-muted-foreground">
                  {t('policy.channels')}
                </th>
                <th className="text-left px-4 py-3 font-medium text-muted-foreground w-44">
                  {t('policy.action')}
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr
                  key={item.key}
                  className={`border-b last:border-0 transition-opacity ${
                    item.action === 'ignore' || item.action === 'silent' ? 'opacity-50' : ''
                  }`}
                >
                  {/* Mail type */}
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      <TypeIcon mailKey={item.key} />
                      <span className="font-medium">{item.name}</span>
                      {item.is_system && (
                        <Badge variant="secondary" className="text-xs px-1.5 py-0">
                          System
                        </Badge>
                      )}
                    </div>
                  </td>

                  {/* Priority */}
                  <td className="px-4 py-3">
                    <Badge variant={priorityVariant(item.priority)} className="text-xs">
                      {priorityLabel(item.priority)}
                    </Badge>
                  </td>

                  {/* Channels checkboxes */}
                  <td className="px-4 py-3">
                    {item.action === 'notify' ? (
                      <div className="flex flex-wrap gap-3">
                        {item.channels.length === 0 ? (
                          <span className="text-muted-foreground text-xs">
                            {t('policy.no_channels')}
                          </span>
                        ) : (
                          item.channels.map((ch) => (
                            <label
                              key={ch.id}
                              className="flex items-center gap-1.5 cursor-pointer select-none"
                            >
                              <Checkbox
                                checked={ch.selected}
                                onCheckedChange={() => handleToggleChannel(item, ch.id)}
                                disabled={saving === item.key}
                              />
                              <span className="text-sm">{ch.name}</span>
                            </label>
                          ))
                        )}
                      </div>
                    ) : (
                      <span className="text-muted-foreground text-xs">—</span>
                    )}
                  </td>

                  {/* Action select */}
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      <Select
                        value={item.action}
                        onValueChange={(v) => handleActionChange(item, v)}
                        disabled={saving === item.key}
                      >
                        <SelectTrigger className="w-32 h-8">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="notify">{t('policy.action_notify')}</SelectItem>
                          <SelectItem value="silent">{t('policy.action_silent')}</SelectItem>
                          <SelectItem value="ignore">{t('policy.action_ignore')}</SelectItem>
                        </SelectContent>
                      </Select>
                      {saving === item.key && (
                        <RefreshCw className="h-3.5 w-3.5 animate-spin text-muted-foreground" />
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
