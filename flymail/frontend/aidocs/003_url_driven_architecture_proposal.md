# 003 - URL驱动架构重构提案

## 背景

当前邮箱应用采用的是"状态驱动"模式（State → URL），而主流Web邮箱应用采用"URL驱动"模式（URL → State）。为了提供更好的用户体验和符合Web标准，建议进行架构重构。

## 当前架构 vs 目标架构

### 当前架构：状态驱动
```
用户操作 → 应用状态更新 → 防抖后URL更新
         ↓
      UI立即刷新
```

### 目标架构：URL驱动
```
用户操作 → URL更新 → 路由监听 → 状态解析 → UI刷新
```

## 实施计划

### 阶段1：准备工作（1-2天）
- [ ] 设计新的URL结构规范
- [ ] 创建URL构建和解析工具函数
- [ ] 建立路由到状态的映射关系

### 阶段2：核心重构（3-5天）
- [ ] 重构状态管理器，以URL为主导
- [ ] 修改所有用户操作入口点
- [ ] 实现URL变化的统一处理逻辑

### 阶段3：优化完善（1-2天）
- [ ] 性能优化和防抖处理
- [ ] 错误处理和降级方案
- [ ] 浏览器兼容性测试

### 阶段4：测试验证（1天）
- [ ] 功能测试：所有操作通过URL驱动
- [ ] 浏览器历史测试：前进/后退正确工作
- [ ] 深度链接测试：直接访问任意状态URL
- [ ] 性能测试：确保响应速度不受影响

## 详细设计

### URL结构设计
```
基础格式：/mail?a={accountId}&f={folderId}&m={mailId}&view={viewState}

示例：
- 虚拟文件夹：/mail?a=0&f=all-inbox
- 普通文件夹：/mail?a=3&f=30
- 邮件详情：/mail?a=3&f=30&m=1234
- 撰写邮件：/mail?a=3&f=30&view=compose&compose=new
- 设置页面：/mail?view=settings
```

### 核心组件重构

#### 1. URL工具函数
```typescript
// utils/urlHelper.ts
export const buildMailUrl = (state: AppState): string => {
  const params = new URLSearchParams()

  if (state.accountId !== undefined) {
    params.set('a', String(state.accountId === null ? 0 : state.accountId))
  }

  if (state.virtualFolder) {
    params.set('f', state.virtualFolder)
  } else if (state.folderId) {
    params.set('f', String(state.folderId))
  }

  if (state.mailId) {
    params.set('m', String(state.mailId))
  }

  // ... 其他参数

  return `/mail?${params.toString()}`
}

export const parseMailUrl = (url: string): Partial<AppState> => {
  const params = new URLSearchParams(new URL(url).search)
  // ... 解析逻辑
}
```

#### 2. 路由驱动的状态管理器
```typescript
// composables/useUrlDrivenState.ts
export const useUrlDrivenState = () => {
  const router = useRouter()
  const route = useRoute()

  // 用户操作统一入口：更新URL
  const updateState = (updates: Partial<AppState>) => {
    const currentState = parseMailUrl(route.fullPath)
    const newState = { ...currentState, ...updates }
    const newUrl = buildMailUrl(newState)

    // 使用 router.push 支持浏览器历史
    router.push(newUrl)
  }

  // URL变化监听：解析并更新应用状态
  watch(() => route.query, (newQuery) => {
    const urlState = parseMailUrl(route.fullPath)
    syncStateToStores(urlState)
  }, { immediate: true })

  return {
    updateState,
    selectAccount: (id: number) => updateState({ accountId: id }),
    selectFolder: (id: number) => updateState({ folderId: id }),
    selectMail: (id: number) => updateState({ mailId: id }),
    // ...
  }
}
```

#### 3. 组件更新示例
```typescript
// AccountTreeApi.vue
function selectFolder(folderId: number, accountId?: number) {
  if (stateManager) {
    // 直接更新URL，让路由系统处理状态变化
    if (accountId && accountId !== mailStore.selectedAccountId) {
      stateManager.updateState({ accountId, folderId })
    } else {
      stateManager.updateState({ folderId })
    }
  }
}
```

## 优势分析

### 用户体验
- ✅ 浏览器前进/后退完美支持
- ✅ 深度链接绝对准确
- ✅ 支持书签和分享
- ✅ 多标签页状态独立

### 开发体验
- ✅ 调试更容易：URL直接反映状态
- ✅ 测试更简单：可以直接构造URL测试
- ✅ 符合Web标准：REST风格的状态管理

### 技术优势
- ✅ SEO友好：每个状态都有对应URL
- ✅ 缓存友好：不同状态有不同URL
- ✅ 分析友好：便于用户行为分析

## 风险评估与缓解

### 主要风险
1. **重构工作量大**：涉及多个核心组件
2. **性能影响**：频繁的路由变化可能影响性能
3. **兼容性问题**：需要处理各种边界情况

### 缓解措施
1. **渐进式重构**：分阶段实施，降低风险
2. **性能优化**：
   - 使用 `router.replace` 而非 `push` 避免历史堆积
   - 实现智能防抖，避免过度更新
   - 预加载关键状态数据
3. **降级方案**：保留当前逻辑作为fallback

## 实施时间估算

- **准备阶段**：1-2天
- **核心重构**：3-5天
- **测试优化**：2-3天
- **总计**：6-10天

## 成功标准

### 功能标准
- [ ] 所有用户操作都通过URL驱动
- [ ] 浏览器前进/后退完全正常
- [ ] 深度链接100%准确
- [ ] 页面刷新后状态完全恢复

### 性能标准
- [ ] 操作响应时间不超过当前方案的120%
- [ ] 内存使用不增加超过10%
- [ ] 包体积增加不超过5%

### 质量标准
- [ ] 零功能回归
- [ ] 代码覆盖率不低于当前水平
- [ ] 通过所有现有测试用例

## 后续规划

重构完成后的扩展可能：
1. **书签功能**：用户可以收藏常用的邮件状态
2. **分享功能**：轻松分享特定邮件或文件夹
3. **协作功能**：多人查看相同的邮件状态
4. **高级导航**：面包屑导航、快速跳转等

## 结论

URL驱动架构是现代Web应用的标准模式，虽然重构成本较高，但长远来看对用户体验、开发效率和产品功能扩展都有显著好处。建议尽快启动重构计划。