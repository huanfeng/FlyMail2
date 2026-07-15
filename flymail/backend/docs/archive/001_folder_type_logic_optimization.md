# 文件夹类型判断逻辑优化

## 问题描述

在 `internal/api/handler/folder.go` 中的 `determineFolderType` 函数和 `internal/store/model/folder_type.go` 中的 `DetermineFolderType` 函数存在重复的逻辑。

## 现状分析

### 原始实现

1. **`model.DetermineFolderType`** 函数:
   - 使用精确匹配（switch 语句）
   - 支持常见的英文和中文文件夹名称
   - 返回 `FolderTypeCustom` 作为默认值

2. **`determineFolderType`** 函数:
   - 首先检查 IMAP flags（最可靠）
   - 调用 `model.DetermineFolderType`
   - 如果返回 `FolderTypeCustom`，则进行更详细的模糊匹配
   - 支持更多中文名称变体和 `rawName` 参数

### 重复逻辑

两个函数都有相同的英文和中文名称匹配逻辑，但 `determineFolderType` 提供了更强的匹配能力。

## 优化方案

### 1. 创建增强版函数

在 `internal/store/model/folder_type.go` 中创建新的 `DetermineFolderTypeAdvanced` 函数：

```go
// DetermineFolderTypeAdvanced determines folder type with advanced matching
// 支持模糊匹配和 rawName 参数
func DetermineFolderTypeAdvanced(folderName, rawName string) FolderType
```

### 2. 功能特性

- **精确匹配优先**: 使用 switch 语句进行精确匹配
- **模糊匹配支持**: 使用 `strings.Contains` 进行模糊匹配
- **rawName 支持**: 同时检查解码后的名称和原始名称
- **更多中文变体**: 支持更多中文文件夹名称变体
- **向后兼容**: 保持原有 `DetermineFolderType` 函数的兼容性

### 3. 简化现有函数

`determineFolderType` 函数简化为：

```go
func determineFolderType(name, rawName string, flags []string) model.FolderType {
	// Check by flags first (most reliable)
	for _, flag := range flags {
		// ... flag 检查逻辑 ...
	}

	// Use the enhanced folder type determination with advanced matching
	return model.DetermineFolderTypeAdvanced(name, rawName)
}
```

## 改进内容

### 1. 统一逻辑

- 将重复的文件夹类型判断逻辑合并到 `model.DetermineFolderTypeAdvanced` 函数中
- 避免代码重复，提高维护性

### 2. 增强匹配能力

- 支持精确匹配和模糊匹配
- 同时检查 `folderName` 和 `rawName` 参数
- 支持更多中文文件夹名称变体

### 3. 更新调用点

更新了两个使用 `model.DetermineFolderType` 的地方：

1. `internal/api/handler/folder.go` - 使用 `DetermineFolderTypeAdvanced(name, rawName)`
2. `internal/worker/sync/handler.go` - 使用 `DetermineFolderTypeAdvanced(decodedFolderName, folderName)`

## 测试验证

创建了完整的测试套件 `internal/store/model/folder_type_test.go`，包括：

- 精确匹配测试
- 模糊匹配测试
- 中文变体测试
- rawName 测试
- 自定义文件夹测试
- 向后兼容性测试

所有测试均通过，确保优化后的逻辑正确工作。

## 优化效果

1. **代码重复减少**: 消除了重复的文件夹类型判断逻辑
2. **维护性提升**: 统一的逻辑更容易维护和扩展
3. **匹配能力增强**: 支持更多文件夹名称变体和模糊匹配
4. **向后兼容**: 保持现有 API 的兼容性
5. **测试覆盖**: 完整的测试套件确保逻辑正确性

## 文件修改清单

1. `internal/store/model/folder_type.go` - 新增 `DetermineFolderTypeAdvanced` 函数
2. `internal/api/handler/folder.go` - 简化 `determineFolderType` 函数
3. `internal/worker/sync/handler.go` - 更新使用新函数
4. `internal/store/model/folder_type_test.go` - 新增测试文件

## 结论

通过这次优化，我们成功地合并了重复的文件夹类型判断逻辑，提高了代码的可维护性和匹配能力，同时保持了向后兼容性。