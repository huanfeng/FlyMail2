export type Theme = 'light' | 'dark'
const KEY = 'flymail_theme'

export function getTheme(): Theme {
  return localStorage.getItem(KEY) === 'dark' ? 'dark' : 'light'
}

export function applyTheme(t: Theme): void {
  localStorage.setItem(KEY, t)
  document.documentElement.classList.toggle('dark', t === 'dark')
}
