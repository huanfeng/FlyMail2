// 主题管理工具函数

export const themes = [
  { name: 'zinc', label: 'Zinc', color: 'hsl(240 6% 10%)' },
  { name: 'slate', label: 'Slate', color: 'hsl(222 47% 11%)' },
  { name: 'stone', label: 'Stone', color: 'hsl(25 5% 45%)' },
  { name: 'gray', label: 'Gray', color: 'hsl(220 9% 46%)' },
  { name: 'neutral', label: 'Neutral', color: 'hsl(0 0% 45%)' },
  { name: 'red', label: 'Red', color: 'hsl(0 72% 51%)' },
  { name: 'rose', label: 'Rose', color: 'hsl(350 89% 60%)' },
  { name: 'orange', label: 'Orange', color: 'hsl(25 95% 53%)' },
  { name: 'green', label: 'Green', color: 'hsl(142 71% 45%)' },
  { name: 'blue', label: 'Blue', color: 'hsl(217 91% 60%)' },
  { name: 'yellow', label: 'Yellow', color: 'hsl(45 93% 47%)' },
  { name: 'violet', label: 'Violet', color: 'hsl(263 70% 50%)' },
]

export function initializeTheme() {
  // 初始化主题
  const savedTheme = localStorage.getItem('selectedTheme') || 'zinc'
  const savedRadius = localStorage.getItem('selectedRadius') || '0.5'
  const savedMode = localStorage.getItem('colorMode') || 'light'
  
  // 应用主题类
  document.documentElement.classList.add(`theme-${savedTheme}`)
  
  // 应用圆角
  document.documentElement.style.setProperty('--radius', `${savedRadius}rem`)
  
  // 应用颜色模式
  if (savedMode === 'dark') {
    document.documentElement.classList.add('dark')
  }
  
  console.log('Theme system initialized:', {
    theme: savedTheme,
    radius: savedRadius,
    mode: savedMode
  })
}

export function debugTheme() {
  const classList = document.documentElement.classList.toString()
  const computedStyle = getComputedStyle(document.documentElement)
  
  console.log('=== Theme debug information ===')
  console.log('HTML class names:', classList)
  console.log('CSS variables:')
  console.log('  --primary:', computedStyle.getPropertyValue('--primary'))
  console.log('  --background:', computedStyle.getPropertyValue('--background'))
  console.log('  --foreground:', computedStyle.getPropertyValue('--foreground'))
  console.log('  --radius:', computedStyle.getPropertyValue('--radius'))
  console.log('LocalStorage:')
  console.log('  selectedTheme:', localStorage.getItem('selectedTheme'))
  console.log('  selectedRadius:', localStorage.getItem('selectedRadius'))
  console.log('  colorMode:', localStorage.getItem('colorMode'))
}

// 扩展Window接口以包含调试函数
declare global {
  interface Window {
    debugTheme?: typeof debugTheme
  }
}

// 在浏览器控制台暴露调试函数
if (typeof window !== 'undefined') {
  window.debugTheme = debugTheme
}