# URL 状态管理系统

## 架构设计

采用单向数据流设计，确保状态同步的一致性：

```
URL (真相源) → StateManager → Stores → UI
                     ↑
                     └─ UI操作触发更新
```

## 核心原则

1. **URL 为真相源**：刷新页面时，URL参数优先级最高
2. **单向数据流**：避免循环更新
3. **集中管理**：所有状态变更通过 StateManager
4. **防止竞争**：使用标志位防止状态更新循环

## 使用方法

### URL 参数说明

- `a` - 账户ID
- `f` - 文件夹ID  
- `m` - 邮件ID
- `view` - 视图状态（逗号分隔）：`settings`, `compose`
- `compose` - 撰写模式：`new`, `reply`, `replyAll`, `forward`
- `composeId` - 撰写相关的邮件ID

### 示例 URL

```
#/?a=2&f=34                         // 选择账户2的文件夹34
#/?a=2&f=34&m=123                   // 同时打开邮件123
#/?view=settings                    // 打开设置弹窗
#/?view=compose&compose=new         // 打开新建邮件
#/?view=compose&compose=reply&composeId=123  // 回复邮件123
```

## 代码使用

```typescript
// 在组件中使用
const stateManager = useStateManager()

// 选择账户
stateManager.selectAccount(2)

// 选择文件夹
stateManager.selectFolder(34)

// 打开撰写邮件
stateManager.openCompose('new')
stateManager.openCompose('reply', 123)

// 打开设置
stateManager.openSettings()
```

## 初始化流程

1. 先从URL恢复状态 (`initializeFromUrl`)
2. 初始化store，不会覆盖URL状态
3. 设置监听器 (`setupStoreWatchers`)

## 注意事项

- 从URL恢复时，会跳过账户的自动文件夹选择
- 所有状态更新都会同步到URL（使用replace避免历史记录）
- 使用防护标志避免循环更新