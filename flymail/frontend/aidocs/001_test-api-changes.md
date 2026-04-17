# API 更改测试报告

## 完成的修改

### 1. API 类型定义更新 ✅
- `User.id` → `User.user_id`
- `EmailAccount.id` → `EmailAccount.account_id`
- `Folder.id` → `Folder.folder_id`
- `Email.id` → `Email.email_id`
- `Email.email_id` → `Email.message_id` (邮件唯一标识)
- `Attachment.id` → `Attachment.attachment_id`
- `ScheduledTask.id` → `ScheduledTask.scheduled_task_id`

### 2. 新增批量操作类型 ✅
- `BatchUpdateFlagsRequest` - 批量更新邮件标记
- `BatchDeleteRequest` - 批量删除邮件
- `BatchOperationResult` - 批量操作结果
- `VirtualFolder` - 虚拟文件夹类型

### 3. API 端点更新 ✅
- `/emails/batch/flags` - 批量更新邮件标记
- `/emails/batch` - 批量删除邮件

### 4. 邮件服务更新 ✅
- 更新 `EmailListParams` 支持 `virtual_folder` 参数
- 添加新的批量操作方法
- 保留向后兼容的旧方法

### 5. 前端组件更新 ✅
- `MailList.vue` - 更新所有 `mail.id` → `mail.email_id`
- `MailDisplay.vue` - 更新所有 ID 字段引用
- `useEmails.ts` - 更新邮件 ID 字段引用
- `useFolders.ts` - 更新文件夹 ID 字段引用
- `mailApi.ts` - 更新存储中的 ID 字段引用

## 主要功能增强

### 1. 批量删除邮件
```typescript
// 使用新的 API 端点
await emailsService.batchDelete([1, 2, 3, 4, 5])

// 返回详细的操作结果
{
  successful: 4,
  failed: 1,
  failed_ids: [3],
  errors: ["email not found"]
}
```

### 2. 批量更新邮件标记
```typescript
// 批量标记为已读
await emailsService.batchUpdateFlags([1, 2, 3], { is_read: true })

// 批量添加星标
await emailsService.batchUpdateFlags([4, 5, 6], { is_starred: true })
```

### 3. 虚拟文件夹支持
```typescript
// 获取所有收件箱邮件
await emailsService.getEmails({ virtual_folder: 'all-inbox' })

// 获取所有星标邮件
await emailsService.getEmails({ virtual_folder: 'all-starred' })
```

## 测试需要验证的功能

### 邮件操作
- [ ] 邮件列表显示正常
- [ ] 点击邮件能正确选择和显示详情
- [ ] 星标/取消星标功能正常
- [ ] 标记已读/未读功能正常
- [ ] 删除单个邮件功能正常
- [ ] 批量删除邮件功能正常
- [ ] 批量标记邮件功能正常

### 文件夹操作
- [ ] 文件夹列表显示正常
- [ ] 切换文件夹功能正常
- [ ] 文件夹计数显示正确

### 账户操作
- [ ] 账户列表显示正常
- [ ] 切换账户功能正常
- [ ] 同步账户功能正常

## 兼容性说明

所有修改都保持了向后兼容：
- 新的批量操作 API 是额外添加的，不影响现有单个操作
- 旧的批量操作方法被标记为 `@deprecated` 但仍然可用
- ID 字段名称变更在类型定义中完成，运行时行为保持一致

## 错误处理

新的批量操作 API 提供了更详细的错误信息：
- 明确标识成功和失败的操作数量
- 返回失败的具体邮件 ID 列表
- 提供详细的错误信息数组