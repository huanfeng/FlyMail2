<script lang="ts" setup>
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Clock, Plus, RefreshCw, MoreVertical, Edit2, Trash2, Play, Power, PowerOff } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { useToast } from '@/components/ui/toast'
import TaskConfigDialog from './TaskConfigDialog.vue'
import type { TaskConfig, TaskType, TaskPriority } from '@/api/types'
import { tasksService } from '@/api/services/tasks'

const { t } = useI18n()
const { toast } = useToast()

// 状态管理
const tasks = ref<TaskConfig[]>([])
const isLoading = ref(false)
const error = ref<string | null>(null)
const isRefreshing = ref(false)

// 对话框状态
const dialogOpen = ref(false)
const taskToEdit = ref<TaskConfig | null>(null)
const deleteDialogOpen = ref(false)
const taskToDelete = ref<TaskConfig | null>(null)
const isDeleting = ref(false)
const toggleDialogOpen = ref(false)
const taskToToggle = ref<TaskConfig | null>(null)
const isToggling = ref<number | null>(null)
const isExecuting = ref<number | null>(null)

// 计算属性：活跃任务数量
const activeTasksCount = computed(() => {
  return tasks.value.filter(t => t.is_active).length
})

// 计算属性：任务类型数量
const taskTypesCount = computed(() => {
  return new Set(tasks.value.map(t => t.task_type)).size
})

// 加载任务列表
async function loadTasks() {
  isLoading.value = true
  error.value = null
  try {
    const data = await tasksService.getTaskConfigs()
    tasks.value = data.list
  } catch (err: any) {
    error.value = err.message || t('settings.tasks.loadFailed')
  } finally {
    isLoading.value = false
  }
}

// 刷新列表
async function refreshTasks() {
  isRefreshing.value = true
  await loadTasks()
  isRefreshing.value = false
}

// 添加任务
function handleAdd() {
  taskToEdit.value = null
  dialogOpen.value = true
}

// 编辑任务
function handleEdit(task: TaskConfig) {
  taskToEdit.value = task
  dialogOpen.value = true
}

// 任务保存成功回调
function handleTaskSaved() {
  dialogOpen.value = false
  taskToEdit.value = null
  refreshTasks()
}

// 确认切换任务状态
function confirmToggle(task: TaskConfig) {
  taskToToggle.value = task
  toggleDialogOpen.value = true
}

// 切换任务状态
async function toggleTaskStatus() {
  if (!taskToToggle.value || isToggling.value === taskToToggle.value.id) return

  isToggling.value = taskToToggle.value.id
  try {
    await tasksService.updateTaskConfig(taskToToggle.value.id, {
      is_active: !taskToToggle.value.is_active
    })
    await refreshTasks()
    toggleDialogOpen.value = false
    toast({
      title: t('common.success'),
      description: taskToToggle.value.is_active ? t('settings.tasks.disabled') : t('settings.tasks.enabled')
    })
    taskToToggle.value = null
  } catch (err: any) {
    error.value = err.message || t('settings.tasks.updateFailed')
  } finally {
    isToggling.value = null
  }
}

// 确认删除任务
function confirmDelete(task: TaskConfig) {
  taskToDelete.value = task
  deleteDialogOpen.value = true
}

// 删除任务
async function deleteTask() {
  if (!taskToDelete.value || isDeleting.value) return

  isDeleting.value = true
  try {
    await tasksService.deleteTaskConfig(taskToDelete.value.id)
    await refreshTasks()
    deleteDialogOpen.value = false
    toast({
      title: t('common.success'),
      description: t('settings.tasks.deleted')
    })
    taskToDelete.value = null
  } catch (err: any) {
    error.value = err.message || t('settings.tasks.deleteFailed')
  } finally {
    isDeleting.value = false
  }
}

// 执行任务
async function executeTask(task: TaskConfig) {
  if (isExecuting.value === task.id) return
  
  isExecuting.value = task.id
  try {
    await tasksService.executeTaskConfig(task.id)
    toast({
      title: t('common.success'),
      description: t('settings.tasks.executed')
    })
  } catch (err: any) {
    error.value = err.message || t('settings.tasks.executeFailed')
  } finally {
    isExecuting.value = null
  }
}

// 获取任务类型的显示名称
function getTaskTypeLabel(type: TaskType): string {
  return t(`settings.tasks.types.${type}`)
}

// 获取优先级的显示名称和样式
function getPriorityBadge(priority: TaskPriority) {
  const variants = {
    high: 'destructive',
    normal: 'default',
    low: 'secondary'
  }
  return {
    label: t(`settings.tasks.priorities.${priority}`),
    variant: variants[priority] as any
  }
}

// 获取状态徽章
function getStatusBadge(task: TaskConfig) {
  if (task.is_active) {
    return { text: t('common.enabled'), variant: 'default' as const }
  } else {
    return { text: t('common.disabled'), variant: 'secondary' as const }
  }
}

// 格式化间隔时间
function formatInterval(seconds?: number): string {
  if (!seconds) return '-'
  
  if (seconds < 60) {
    return t('settings.tasks.intervals.seconds', { count: seconds })
  } else if (seconds < 3600) {
    return t('settings.tasks.intervals.minutes', { count: Math.floor(seconds / 60) })
  } else {
    return t('settings.tasks.intervals.hours', { count: Math.floor(seconds / 3600) })
  }
}

// 格式化任务配置显示
function formatTaskConfig(task: TaskConfig): string {
  switch (task.task_type) {
    case 'loop':
      return t('settings.tasks.executionInterval') + ': ' + formatInterval(task.loop_interval)
    case 'scheduled':
      return t('settings.tasks.cronExpression') + ': ' + (task.cron_expression || '-')
    case 'once':
      return t('settings.tasks.types.once')
    default:
      return '-'
  }
}

// 组件挂载时加载数据
onMounted(() => {
  loadTasks()
})
</script>

<template>
  <div class="space-y-6">
    <!-- 头部操作栏 -->
    <div class="flex items-center justify-between">
      <h3 class="text-lg font-medium">{{ t('settings.tasks.title') }}</h3>
      <div class="flex gap-2">
        <Button variant="outline" size="sm" @click="refreshTasks" :disabled="isRefreshing">
          <RefreshCw :class="{ 'animate-spin': isRefreshing }" class="h-4 w-4 mr-2" />
          {{ t('common.refresh') }}
        </Button>
        <Button size="sm" @click="handleAdd">
          <Plus class="h-4 w-4 mr-2" />
          {{ t('settings.tasks.createTask') }}
        </Button>
      </div>
    </div>

    <Separator />

    <!-- 统计信息 -->
    <div class="grid gap-4 md:grid-cols-3">
      <Card>
        <CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle class="text-sm font-medium">{{ t('settings.tasks.totalTasks') }}</CardTitle>
          <Clock class="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div class="text-2xl font-bold">{{ tasks.length }}</div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle class="text-sm font-medium">{{ t('settings.tasks.activeTasks') }}</CardTitle>
          <Power class="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div class="text-2xl font-bold">{{ activeTasksCount }}</div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle class="text-sm font-medium">{{ t('settings.tasks.taskTypes') }}</CardTitle>
          <Clock class="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div class="text-2xl font-bold">{{ taskTypesCount }}</div>
        </CardContent>
      </Card>
    </div>

    <!-- 错误提示 -->
    <Alert v-if="error" variant="destructive">
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>

    <!-- 加载状态 -->
    <div v-if="isLoading" class="space-y-3">
      <Skeleton class="h-20 w-full" />
      <Skeleton class="h-20 w-full" />
      <Skeleton class="h-20 w-full" />
    </div>

    <!-- 任务列表 -->
    <div v-else-if="tasks.length > 0" class="space-y-3">
      <div
        v-for="task in tasks"
        :key="task.id"
        class="rounded-lg border p-4 hover:bg-accent/50 transition-colors"
      >
        <div class="flex items-start justify-between">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 mb-2">
              <div class="font-medium truncate">{{ task.name }}</div>
              <Badge :variant="'outline'">
                {{ getTaskTypeLabel(task.task_type) }}
              </Badge>
              <Badge :variant="getPriorityBadge(task.priority).variant">
                {{ getPriorityBadge(task.priority).label }}
              </Badge>
              <Badge :variant="getStatusBadge(task).variant">
                {{ getStatusBadge(task).text }}
              </Badge>
            </div>
            <div v-if="task.description" class="text-sm text-muted-foreground mb-1">
              {{ task.description }}
            </div>
            <div class="text-sm text-muted-foreground">
              {{ formatTaskConfig(task) }}
              <span v-if="task.last_run" class="ml-2">
                | {{ t('settings.tasks.lastRun') }}: {{ new Date(task.last_run).toLocaleString() }}
              </span>
            </div>
          </div>

          <DropdownMenu>
            <DropdownMenuTrigger as-child>
              <Button variant="ghost" size="icon" class="h-8 w-8">
                <MoreVertical class="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem @click="executeTask(task)" :disabled="isExecuting === task.id || !task.is_active">
                <Play class="mr-2 h-4 w-4" />
                {{ t('settings.tasks.execute') }}
              </DropdownMenuItem>
              <DropdownMenuItem @click="handleEdit(task)">
                <Edit2 class="mr-2 h-4 w-4" />
                {{ t('common.edit') }}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem @click="confirmToggle(task)" :disabled="isToggling === task.id">
                <Power v-if="!task.is_active" class="mr-2 h-4 w-4" />
                <PowerOff v-else class="mr-2 h-4 w-4" />
                {{ task.is_active ? t('common.disable') : t('common.enable') }}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                @click="confirmDelete(task)"
                class="text-destructive focus:text-destructive"
              >
                <Trash2 class="mr-2 h-4 w-4" />
                {{ t('common.delete') }}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    </div>

    <!-- 空状态 -->
    <div v-else class="text-center py-12">
      <Clock class="mx-auto h-12 w-12 text-muted-foreground mb-4" />
      <h3 class="text-lg font-medium mb-2">{{ t('settings.tasks.noTasks') }}</h3>
      <p class="text-sm text-muted-foreground mb-4">
        {{ t('settings.tasks.noTasksDesc') }}
      </p>
      <Button size="sm" @click="handleAdd">
        <Plus class="h-4 w-4 mr-2" />
        {{ t('settings.tasks.createFirstTask') }}
      </Button>
    </div>

    <!-- 任务配置对话框 -->
    <TaskConfigDialog
      v-model:open="dialogOpen"
      :task="taskToEdit"
      @saved="handleTaskSaved"
    />

    <!-- 删除确认对话框 -->
    <AlertDialog v-model:open="deleteDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('settings.tasks.confirmDeleteTitle') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('settings.tasks.confirmDeleteDesc', { name: taskToDelete?.name }) }}
            <br />
            <strong>{{ t('settings.tasks.confirmDeleteWarning') }}</strong>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction
            @click="deleteTask"
            :disabled="isDeleting"
            class="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {{ isDeleting ? t('common.deleting') : t('common.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <!-- 启用/禁用确认对话框 -->
    <AlertDialog v-model:open="toggleDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {{ taskToToggle?.is_active ? t('settings.tasks.confirmDisable') : t('settings.tasks.confirmEnable') }}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {{ taskToToggle?.is_active 
              ? t('settings.tasks.confirmDisableDesc')
              : t('settings.tasks.confirmEnableDesc')
            }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction @click="toggleTaskStatus" :disabled="isToggling === taskToToggle?.id">
            {{ isToggling === taskToToggle?.id ? t('common.processing') : t('common.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>