/**
 * 本地存储管理工具
 * 统一管理所有 localStorage 操作，使用 mail2im_ 前缀
 * 提供密码加密存储功能
 */

// ============================================
// 常量定义
// ============================================

/**
 * LocalStorage 前缀
 * 统一使用 mail2im_ 作为所有 localStorage key 的前缀
 */
export const STORAGE_PREFIX = 'mail2im_';

/**
 * 加密密钥 - 用于密码加密
 * 16个字符固定密钥，更新此密钥将使所有已存储的密码失效
 */
export const ENCRYPTION_KEY = 'mail2im_a1b2c3d4e5f6g7h8';

/**
 * 所有 LocalStorage Key 的集中定义
 * 使用全小写字母+下划线命名规范
 */

// 认证相关
export const KEYS = {
  // 登录记忆（加密存储）
  AUTH_REMEMBER: `${STORAGE_PREFIX}auth_remember`,
  // 会话信息
  AUTH_SESSION: `${STORAGE_PREFIX}auth_session`,

  // 用户界面设置
  UI_LANGUAGE: `${STORAGE_PREFIX}ui_language`,
  UI_THEME_DARK: `${STORAGE_PREFIX}ui_theme_dark`,
  UI_THEME_PRIMARY: `${STORAGE_PREFIX}ui_theme_primary`,
  UI_THEME_SURFACE: `${STORAGE_PREFIX}ui_theme_surface`,

  // 表格设置
  TABLE_ROWS_LOGS: `${STORAGE_PREFIX}table_rows_logs`,
  TABLE_ROWS_EMAILS: `${STORAGE_PREFIX}table_rows_emails`,
  TABLE_ROWS_ACCOUNTS: `${STORAGE_PREFIX}table_rows_accounts`,
  TABLE_ROWS_PROXIES: `${STORAGE_PREFIX}table_rows_proxies`,
  TABLE_ROWS_CHANNELS: `${STORAGE_PREFIX}table_rows_channels`
} as const;

// ============================================
// 密码加密/解密
// ============================================

/**
 * 简单加密函数（基于 XOR）
 * 注意：这是简单加密，用于防止明文存储，非军用级别加密
 * @param text 要加密的文本
 * @param key 加密密钥
 * @returns 加密后的字符串
 */
function simpleEncrypt(text: string, key: string): string {
  let result = '';
  for (let i = 0; i < text.length; i++) {
    const textChar = text.charCodeAt(i);
    const keyChar = key.charCodeAt(i % key.length);
    result += String.fromCharCode(textChar ^ keyChar);
  }
  // 使用 base64 编码结果
  return btoa(result);
}

/**
 * 简单解密函数
 * @param encryptedText 加密的文本
 * @param key 解密密钥
 * @returns 解密后的字符串
 */
function simpleDecrypt(encryptedText: string, key: string): string {
  try {
    // 解码 base64
    const decoded = atob(encryptedText);
    let result = '';
    for (let i = 0; i < decoded.length; i++) {
      const encodedChar = decoded.charCodeAt(i);
      const keyChar = key.charCodeAt(i % key.length);
      result += String.fromCharCode(encodedChar ^ keyChar);
    }
    return result;
  } catch {
    return '';
  }
}

/**
 * 加密密码
 * @param password 原始密码
 * @returns 加密后的密码字符串
 */
export function encryptPassword(password: string): string {
  return simpleEncrypt(password, ENCRYPTION_KEY);
}

/**
 * 解密密码
 * @param encryptedPassword 加密的密码字符串
 * @returns 原始密码
 */
export function decryptPassword(encryptedPassword: string): string {
  return simpleDecrypt(encryptedPassword, ENCRYPTION_KEY);
}

// ============================================
// 安全的 LocalStorage 操作封装
// ============================================

/**
 * 安全地设置 JSON 数据到 localStorage
 * @param key 存储键
 * @param data 要存储的数据
 */
export function setJSON<T>(key: string, data: T): void {
  try {
    localStorage.setItem(key, JSON.stringify(data));
  } catch (error) {
    console.error('Failed to save to localStorage:', error);
  }
}

/**
 * 安全地从 localStorage 读取 JSON 数据
 * @param key 存储键
 * @param defaultValue 默认值
 * @returns 解析后的数据或默认值
 */
export function getJSON<T>(key: string, defaultValue: T): T {
  try {
    const item = localStorage.getItem(key);
    if (item === null) {
      return defaultValue;
    }
    return JSON.parse(item);
  } catch (error) {
    console.error('Failed to parse localStorage data:', error);
    // 清除损坏的数据
    localStorage.removeItem(key);
    return defaultValue;
  }
}

/**
 * 安全地设置字符串到 localStorage
 * @param key 存储键
 * @param value 要存储的字符串值
 */
export function setString(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch (error) {
    console.error('Failed to save to localStorage:', error);
  }
}

/**
 * 安全地从 localStorage 读取字符串
 * @param key 存储键
 * @param defaultValue 默认值
 * @returns 字符串值或默认值
 */
export function getString(key: string, defaultValue: string = ''): string {
  try {
    const item = localStorage.getItem(key);
    return item ?? defaultValue;
  } catch (error) {
    console.error('Failed to read from localStorage:', error);
    return defaultValue;
  }
}

/**
 * 安全地设置布尔值到 localStorage
 * @param key 存储键
 * @param value 要存储的布尔值
 */
export function setBoolean(key: string, value: boolean): void {
  try {
    localStorage.setItem(key, String(value));
  } catch (error) {
    console.error('Failed to save to localStorage:', error);
  }
}

/**
 * 安全地从 localStorage 读取布尔值
 * @param key 存储键
 * @param defaultValue 默认值
 * @returns 布尔值或默认值
 */
export function getBoolean(key: string, defaultValue: boolean = false): boolean {
  try {
    const item = localStorage.getItem(key);
    return item === null ? defaultValue : item === 'true';
  } catch (error) {
    console.error('Failed to read from localStorage:', error);
    return defaultValue;
  }
}

/**
 * 安全地设置数字到 localStorage
 * @param key 存储键
 * @param value 要存储的数字
 */
export function setNumber(key: string, value: number): void {
  try {
    localStorage.setItem(key, String(value));
  } catch (error) {
    console.error('Failed to save to localStorage:', error);
  }
}

/**
 * 安全地从 localStorage 读取数字
 * @param key 存储键
 * @param defaultValue 默认值
 * @returns 数字值或默认值
 */
export function getNumber(key: string, defaultValue: number = 0): number {
  try {
    const item = localStorage.getItem(key);
    if (item === null) {
      return defaultValue;
    }
    const parsed = Number(item);
    return Number.isNaN(parsed) ? defaultValue : parsed;
  } catch (error) {
    console.error('Failed to read from localStorage:', error);
    return defaultValue;
  }
}

/**
 * 从 localStorage 删除指定键
 * @param key 存储键
 */
export function remove(key: string): void {
  try {
    localStorage.removeItem(key);
  } catch (error) {
    console.error('Failed to remove from localStorage:', error);
  }
}

/**
 * 清空所有 mail2im_ 前缀的数据
 */
export function clearAll(): void {
  try {
    const keysToRemove: string[] = [];
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (key?.startsWith(STORAGE_PREFIX)) {
        keysToRemove.push(key);
      }
    }
    keysToRemove.forEach(key => localStorage.removeItem(key));
  } catch (error) {
    console.error('Failed to clear localStorage:', error);
  }
}

// ============================================
// 特定数据类型的便捷方法
// ============================================

/**
 * 存储加密的登录记忆
 * @param identifier 用户名或邮箱
 * @param password 密码（将被加密）
 */
export function setRememberedCredentials(identifier: string, password: string): void {
  const encryptedPassword = encryptPassword(password);
  setJSON(KEYS.AUTH_REMEMBER, {
    identifier,
    password: encryptedPassword
  });
}

/**
 * 获取解密的登录记忆
 * @returns 包含用户名和密码的对象，如果不存在则返回 null
 */
export function getRememberedCredentials(): { identifier: string; password: string } | null {
  const data = getJSON<{ identifier: string; password: string } | null>(KEYS.AUTH_REMEMBER, null);
  if (!data) return null;

  try {
    return {
      identifier: data.identifier,
      password: decryptPassword(data.password)
    };
  } catch {
    // 解密失败，清除错误数据
    remove(KEYS.AUTH_REMEMBER);
    return null;
  }
}

/**
 * 清除登录记忆
 */
export function clearRememberedCredentials(): void {
  remove(KEYS.AUTH_REMEMBER);
}
