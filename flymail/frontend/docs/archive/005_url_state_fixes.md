# 005 - URL状态管理细节修复

## 修复内容

本次修复解决了URL驱动架构中三个关键问题，确保邮箱应用的状态能够正确反映在URL中，并且页面刷新后能够正确恢复状态。

## 问题1: 邮件选择未反映在URL中

### 问题描述
点击邮件列表中的邮件时，邮件内容会显示，但URL中没有 `m` 参数，页面刷新后邮件选择状态丢失。

### 根本原因
`MailList.vue` 中的 `selectMail()` 函数直接调用 `mailStore.selectEmail()`，绕过了URL驱动状态管理器。

### 修复方案
```typescript
// 修改前
function selectMail(emailId: number) {
  mailStore.selectEmail(emailId)
}

// 修改后
function selectMail(emailId: number) {
  // 优先使用URL驱动状态管理器
  if (stateManager) {
    stateManager.selectMail(emailId)
  } else {
    // 降级处理，直接使用 mailStore
    console.warn('⚠️ [MailList] StateManager未注入，使用降级逻辑')
    mailStore.selectEmail(emailId)
  }
}
```

### 效果
- 选择邮件时URL自动更新为 `#/main?a=1&f=30&m=1234`
- 页面刷新后邮件选择状态保持
- 支持邮件详情的直接链接分享

## 问题2: 设置按钮未反映在URL中

### 问题描述
点击设置按钮时，设置弹窗会打开，但URL中没有 `view=settings` 参数，页面刷新后弹窗状态丢失。

### 根本原因
`UserProfile.vue` 中的 `handleSettingsClick()` 函数直接调用 `settingsStore.openSettings()`，绕过了URL驱动状态管理器。

### 修复方案
```typescript
// 修改前
function handleSettingsClick() {
  settingsTooltipOpen.value = false
  settingsStore.openSettings()
}

// 修改后
function handleSettingsClick() {
  settingsTooltipOpen.value = false

  // 优先使用URL驱动状态管理器
  if (stateManager) {
    stateManager.openSettings()
  } else {
    // 降级处理，直接使用 settingsStore
    console.warn('⚠️ [UserProfile] StateManager未注入，使用降级逻辑')
    settingsStore.openSettings()
  }
}
```

### 效果
- 打开设置时URL自动更新为 `#/main?view=settings`
- 页面刷新后设置弹窗保持打开状态
- 支持设置页面的直接链接访问

## 问题3: 虚拟文件夹刷新时自动选中问题

### 问题描述
当选中虚拟文件夹（如"所有收件箱"）后刷新页面，会自动选中第一个账户的第一个文件夹，导致虚拟文件夹状态丢失。

### 根本原因
存在两处自动选择逻辑没有正确处理虚拟文件夹状态：

1. `useFolders.ts` 中的自动选择收件箱逻辑
2. `mailStore.selectAccountEnhanced()` 中的自动选择逻辑

### 修复方案

#### 修复1: useFolders.ts
```typescript
// 修改前
// Auto-select inbox if no folder selected
if (!selectedFolderId.value && inboxFolder.value) {
  selectedFolderId.value = inboxFolder.value.folder_id
}

// 修改后
// Auto-select inbox if no folder selected and account is valid (not virtual folder state)
if (!selectedFolderId.value && inboxFolder.value && accountId.value !== null) {
  selectedFolderId.value = inboxFolder.value.folder_id
}
```

#### 修复2: mailApi.ts - selectAccountEnhanced
```typescript
// 修改前
if (!skipAutoSelectFolder) {
  const inboxFolder = folders.value.find(f => f.type === 'inbox' && f.account_id === accountId)
  if (inboxFolder) {
    await selectFolderOptimized(inboxFolder.folder_id)
  }
}

// 修改后
if (!skipAutoSelectFolder && !selectedVirtualFolder.value) {
  const inboxFolder = folders.value.find(f => f.type === 'inbox' && f.account_id === accountId)
  if (inboxFolder) {
    await selectFolderOptimized(inboxFolder.folder_id)
  }
}
```

#### 修复3: mailApi.ts - 同账户处理
```typescript
// 修改前
if (!skipAutoSelectFolder && !selectedFolderId.value) {
  const inboxFolder = folders.value.find(f => f.type === 'inbox' && f.account_id === accountId)
  if (inboxFolder) {
    await selectFolderOptimized(inboxFolder.folder_id)
  }
}

// 修改后
if (!skipAutoSelectFolder && !selectedFolderId.value && !selectedVirtualFolder.value) {
  const inboxFolder = folders.value.find(f => f.type === 'inbox' && f.account_id === accountId)
  if (inboxFolder) {
    await selectFolderOptimized(inboxFolder.folder_id)
  }
}
```

### 效果
- 虚拟文件夹状态在页面刷新后正确保持
- 不会误触发自动选择账户文件夹的逻辑
- URL为 `#/main?a=0&f=all-inbox` 时正确显示虚拟文件夹

## 技术细节

### URL参数格式
- `a`: 账户ID (0=虚拟文件夹模式，null=所有账户)
- `f`: 文件夹ID或虚拟文件夹名称
- `m`: 邮件ID
- `view`: 弹窗状态 (settings,compose)

### 状态管理流程
1. **用户操作** → URL驱动状态管理器
2. **URL更新** → 路由监听器
3. **状态解析** → 同步到各个Store
4. **UI更新** → 反映最新状态

### 降级处理
所有组件都实现了降级处理机制：
- 优先使用注入的URL驱动状态管理器
- 如果状态管理器未注入，直接使用对应的Store
- 输出警告日志便于调试

## 测试建议

### 手动测试步骤
1. **邮件选择测试**:
   - 点击邮件 → 检查URL是否包含 `m` 参数
   - 刷新页面 → 邮件选择是否保持

2. **设置弹窗测试**:
   - 点击设置 → 检查URL是否包含 `view=settings`
   - 刷新页面 → 设置弹窗是否保持打开

3. **虚拟文件夹测试**:
   - 选择"所有收件箱" → 检查URL为 `a=0&f=all-inbox`
   - 刷新页面 → 虚拟文件夹选择是否保持
   - 确认不会自动跳转到具体账户文件夹

### 自动化测试
建议添加以下测试用例：
- URL参数解析正确性
- 状态同步的完整性
- 虚拟文件夹状态的持久性
- 降级处理的正确性

## 影响范围

### 修改的文件
- `src/components/mail/MailList.vue`
- `src/components/mail/UserProfile.vue`
- `src/composables/useFolders.ts`
- `src/stores/mailApi.ts`

### 向后兼容性
- 所有修改都是向后兼容的
- 实现了降级处理机制
- 不影响现有功能的正常使用

## 总结

通过这次修复，邮箱应用实现了完整的URL驱动状态管理：
- **邮件选择**、**设置弹窗**、**虚拟文件夹** 状态都正确反映在URL中
- 页面刷新后所有状态都能正确恢复
- 支持直接通过URL访问特定状态
- 提供了可靠的降级处理机制

用户体验得到显著提升，特别是在页面刷新、浏览器前进后退、以及链接分享场景下。