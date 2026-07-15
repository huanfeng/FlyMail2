# 008 - Composables 清理建议

## 清理目标

经过URL驱动架构重构后，一些 composables 文件已经不再使用，需要进行清理以保持代码库的精练性。

## 分析结果

### ✅ 正在使用的文件
1. **useUrlDrivenState.ts** - 当前的主要状态管理器
   - 在 `MailView.vue` 中使用
   - 替代了原来的 `useStateManager`

2. **useEmails.ts** - 邮件管理逻辑
   - 在 `mailApi.ts` 中使用
   - 核心业务逻辑，必须保留

3. **useFolders.ts** - 文件夹管理逻辑
   - 在 `mailApi.ts` 中使用
   - 核心业务逻辑，必须保留

4. **useAccounts.ts** - 账户管理逻辑
   - 在 `mailApi.ts` 中使用
   - 核心业务逻辑，必须保留

5. **useSessionState.ts** - 会话状态管理
   - 在 `MailView.vue` 中使用
   - 用于面板大小等临时状态，必须保留

### ❌ 可以删除的文件

#### 1. useStateManager.ts
**状态**: 已废弃
**原因**:
- 只被用来提供类型定义 `AppState`
- 实际的状态管理逻辑已经迁移到 `useUrlDrivenState`
- 现在组件中注入的 `stateManager` 实际上是 `useUrlDrivenState` 的包装

**影响评估**:
- 需要提取 `AppState` 类型定义到独立文件
- 删除后不影响任何功能

#### 2. useLanguageDetection.ts
**状态**: 完全未使用
**原因**: 没有任何文件导入或使用该 composable
**影响评估**: 删除后无任何影响

#### 3. useTimeAgo.ts
**状态**: 完全未使用
**原因**: 没有任何文件导入或使用该 composable
**影响评估**: 删除后无任何影响

#### 4. usePolling.ts
**状态**: 完全未使用
**原因**: 没有任何文件导入或使用该 composable
**影响评估**: 删除后无任何影响

## 清理步骤

### 第1步: 提取类型定义
创建 `src/types/state.ts` 文件，将 `AppState` 类型从 `useStateManager.ts` 中提取：

```typescript
// src/types/state.ts
export interface AppState {
  accountId?: number | null
  folderId?: number | null
  virtualFolder?: string | null
  mailId?: number
  composeOpen: boolean
  composeMode: 'new' | 'reply' | 'replyAll' | 'forward'
  composeMailId?: number
  settingsOpen: boolean
}
```

### 第2步: 更新导入引用
更新以下文件中的类型导入：
- `src/composables/useUrlDrivenState.ts`
- `src/utils/urlHelper.ts`

```typescript
// 修改前
import type { AppState } from './useStateManager'

// 修改后
import type { AppState } from '@/types/state'
```

### 第3步: 删除废弃文件
删除以下文件：
- `src/composables/useStateManager.ts`
- `src/composables/useLanguageDetection.ts`
- `src/composables/useTimeAgo.ts`
- `src/composables/usePolling.ts`

### 第4步: 清理组件类型引用
更新组件中的类型引用，从：
```typescript
import type { useStateManager } from '@/composables/useStateManager'
const stateManager = inject<ReturnType<typeof useStateManager>>('stateManager')!
```

改为：
```typescript
import type { UrlDrivenStateManager } from '@/composables/useUrlDrivenState'
const stateManager = inject<UrlDrivenStateManager>('stateManager')!
```

## 架构优化

### 当前架构问题
现在的架构存在一层不必要的包装：
1. `MailView.vue` 使用 `useUrlDrivenState`
2. 然后包装成类似 `useStateManager` 的接口 provide 给子组件
3. 子组件 inject 时使用 `useStateManager` 的类型

### 建议优化
直接 provide `urlStateManager` 实例：
```typescript
// MailView.vue
provide('stateManager', urlStateManager)
```

组件中直接使用正确的类型：
```typescript
// 各个组件
import type { UrlDrivenStateManager } from '@/composables/useUrlDrivenState'
const stateManager = inject<UrlDrivenStateManager>('stateManager')!
```

## 预期收益

### 代码减少
- **删除约 600 行**废弃代码
- **减少 4 个文件**
- **简化类型系统**

### 架构优化
- **消除历史包袱**: 删除旧的状态管理器
- **类型一致性**: 使用正确的类型定义
- **代码清晰度**: 减少不必要的抽象层

### 维护优势
- **减少混淆**: 开发者不会意外使用废弃的 composables
- **降低复杂度**: 简化状态管理相关代码
- **提高可读性**: 代码库更加简洁

## 风险评估

### 潜在风险
- 类型定义迁移可能影响构建

### 风险缓解
- 先提取类型定义，再删除文件
- 逐步更新类型引用
- 确保所有组件使用正确的类型

## 总结

这次清理将：
- 删除 4 个废弃的 composables 文件
- 提取类型定义到独立文件
- 优化组件间的类型一致性
- 显著简化状态管理相关代码

符合"在开发阶段优先考虑架构一致性和代码精练性"的原则。