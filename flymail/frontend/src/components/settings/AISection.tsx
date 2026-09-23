// 设置 → AI 翻译（OpenAI 兼容接口配置）。
//
// ── 为什么只有三个字段 ─────────────────────────────────────────────────────
//
// 地址、模型、密钥——OpenAI 兼容接口本来就只要这三样。多摆一个参数
// （温度、最大长度、超时）就多一处"换个服务商要重新试"的地方，
// 而它们的默认值对翻译都足够好。
//
// ── 为什么把预设地址摆出来 ─────────────────────────────────────────────────
//
// 「OpenAI 兼容」对用户不是一句自明的话：他知道自己买了某家的额度，
// 但不知道该往框里填什么。列几条常见地址，配置这件事就从"去翻文档"
// 变成"照抄一行"。列表不求全，只求覆盖到最常见的几种来路。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { apiErrorMessage } from '@/lib/api'
import { useSettings, useTranslateLanguages, useUpdateSettings } from '@/lib/queries'

/**
 * 常见服务的接口地址。
 *
 * 只列地址不列模型名：模型名各家改得勤，写死在界面上迟早过期，
 * 而过期的模型名比没有更糟——用户照抄之后得到的是一个 404。
 */
const PRESETS: readonly { label: string; url: string }[] = [
  { label: 'OpenAI', url: 'https://api.openai.com/v1' },
  { label: 'DeepSeek', url: 'https://api.deepseek.com/v1' },
  { label: 'Ollama', url: 'http://127.0.0.1:11434/v1' },
]

export function AISection() {
  const { t } = useTranslation()
  const { data: settings } = useSettings()
  const { data: langs } = useTranslateLanguages()
  const update = useUpdateSettings()

  const storedURL = settings?.ai_base_url ?? ''
  const storedModel = settings?.ai_model ?? ''
  const storedLang = settings?.translate_target_lang ?? 'zh'
  const keySaved = settings?.ai_api_key_set ?? false

  const [baseURL, setBaseURL] = React.useState(storedURL)
  const [model, setModel] = React.useState(storedModel)
  const [target, setTarget] = React.useState(storedLang)
  // 空串在这里的意思是「不改动已保存的那份」，不是「清成空」——清除走单独的按钮。
  // 两者混为一谈的话，用户只改模型名点保存就会把密钥连带抹掉。
  const [apiKey, setApiKey] = React.useState('')
  const [error, setError] = React.useState<string | null>(null)
  const [saved, setSaved] = React.useState(false)

  // 服务端数据回来后同步一次；用 ref 记住已同步过的值，免得冲掉用户正在输入的内容
  const syncedRef = React.useRef<string | null>(null)
  React.useEffect(() => {
    if (settings == null) return
    const stamp = `${storedURL}|${storedModel}|${storedLang}`
    if (syncedRef.current === stamp) return
    syncedRef.current = stamp
    setBaseURL(storedURL)
    setModel(storedModel)
    setTarget(storedLang)
  }, [settings, storedURL, storedModel, storedLang])

  function apply(payload: Record<string, string>, onOk?: () => void) {
    setError(null)
    update.mutate(payload, {
      onSuccess: () => {
        onOk?.()
        setSaved(true)
        setTimeout(() => setSaved(false), 2500)
      },
      // 后端的拒绝理由是写给人看的中文，而且是 400——重试多少次都一样。
      onError: (err) => setError(apiErrorMessage(err, t('settings.ai.errSave'))),
    })
  }

  function save() {
    const payload: Record<string, string> = {
      ai_base_url: baseURL.trim(),
      ai_model: model.trim(),
      translate_target_lang: target,
    }
    if (apiKey !== '') payload.ai_api_key = apiKey
    apply(payload, () => setApiKey(''))
  }

  const configured = storedURL !== '' && storedModel !== ''

  return (
    <div className="settings-block">
      <h3>{t('settings.ai.title')}</h3>
      <p className="help">{t('settings.ai.intro')}</p>

      {/* 当前状态摆在最前面：用户来这个页面十有八九是因为翻译按钮没反应，
          第一句话就该回答"到底配没配上"。 */}
      <p className="help" style={{ color: configured ? 'var(--accent)' : 'var(--ink-3)' }}>
        {configured ? t('settings.ai.statusReady', { model: storedModel }) : t('settings.ai.statusEmpty')}
      </p>

      <div className="settings-field">
        <label className="sf-label" htmlFor="ai-base-url">{t('settings.ai.baseUrl')}</label>
        <Input
          id="ai-base-url"
          placeholder="https://api.openai.com/v1"
          value={baseURL}
          onChange={(e) => { setBaseURL(e.target.value); setError(null) }}
        />
        <p className="sf-help">{t('settings.ai.baseUrlHelp')}</p>
        <div className="settings-copybar" style={{ marginTop: 6 }}>
          <span className="sf-help" style={{ margin: 0 }}>{t('settings.ai.presets')}</span>
          {PRESETS.map((p) => (
            <Button
              key={p.url}
              size="sm"
              variant="outline"
              onClick={() => { setBaseURL(p.url); setError(null) }}
            >
              {p.label}
            </Button>
          ))}
        </div>
      </div>

      <div className="settings-field">
        <label className="sf-label" htmlFor="ai-model">{t('settings.ai.model')}</label>
        <Input
          id="ai-model"
          placeholder="gpt-4o-mini"
          value={model}
          onChange={(e) => { setModel(e.target.value); setError(null) }}
        />
        <p className="sf-help">{t('settings.ai.modelHelp')}</p>
      </div>

      <div className="settings-field">
        <label className="sf-label" htmlFor="ai-api-key">{t('settings.ai.apiKey')}</label>
        <div className="settings-copybar">
          <Input
            id="ai-api-key"
            type="password"
            autoComplete="new-password"
            style={{ flex: '1 1 280px', minWidth: 0 }}
            placeholder={keySaved ? t('settings.ai.keySaved') : t('settings.ai.keyEmpty')}
            value={apiKey}
            onChange={(e) => { setApiKey(e.target.value); setError(null) }}
          />
          {keySaved && (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => apply({ ai_api_key: '' }, () => setApiKey(''))}
              disabled={update.isPending}
            >
              {t('settings.ai.clearKey')}
            </Button>
          )}
        </div>
        <p className="sf-help">{t('settings.ai.apiKeyHelp')}</p>
      </div>

      <div className="settings-field">
        <label className="sf-label" htmlFor="ai-target-lang">{t('settings.ai.targetLang')}</label>
        <select
          id="ai-target-lang"
          value={target}
          onChange={(e) => { setTarget(e.target.value); setError(null) }}
        >
          {/* 清单来自后端：语言代码同时用于提示词与校验，前端再维护一份
              迟早会漂移，表现为"选得到却报不支持"。 */}
          {(langs?.languages ?? [{ code: storedLang, name: storedLang, native: storedLang }]).map((l) => (
            <option key={l.code} value={l.code}>{l.native}</option>
          ))}
        </select>
        <p className="sf-help">{t('settings.ai.targetLangHelp')}</p>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 10, paddingTop: 14 }}>
        <Button size="sm" onClick={save} disabled={update.isPending}>
          {t('settings.ai.save')}
        </Button>
        {saved && <span style={{ fontSize: 13, color: 'var(--accent)' }}>{t('settings.ai.saved')}</span>}
      </div>

      {error != null && (
        <div style={{ color: 'var(--destructive)', fontSize: 12, marginTop: 8 }}>{error}</div>
      )}
    </div>
  )
}
