<script lang="ts" setup>
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/toast'
import type { TaskConfig, TaskType, TaskPriority, CreateTaskConfigRequest, UpdateTaskConfigRequest } from '@/api/types'
import { tasksService } from '@/api/services/tasks'

const props = defineProps<{
  open: boolean
  task?: TaskConfig | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  'saved': []
}>()

const { t } = useI18n()
const { toast } = useToast()

const formData = ref({
  task_type: 'loop' as TaskType,
  task_name: '',
  name: '',
  description: '',
  priority: 'normal' as TaskPriority,
  loop_interval: 60,
  cron_expression: '',
  is_active: true
})

const loading = ref(false)
const errors = ref<Record<string, string>>({})

const isEdit = computed(() => !!props.task)
const dialogTitle = computed(() => 
  isEdit.value ? t('settings.tasks.editTask') : t('settings.tasks.createTask')
)

// 监听任务类型变化
watch(() => formData.value.task_type, (newType) => {
  // 根据任务类型设置默认的任务名称
  if (!isEdit.value && !formData.value.task_name) {
    switch (newType) {
      case 'loop':
        formData.value.task_name = 'email_monitor'
        break
      case 'scheduled':
        formData.value.task_name = 'email_sync'
        break
      case 'once':
        formData.value.task_name = 'manual_task'
        break
    }
  }
})

// 监听对话框打开状态
watch(() => props.open, (newValue) => {
  if (newValue) {
    if (props.task) {
      // 编辑模式
      formData.value = {
        task_type: props.task.task_type,
        task_name: props.task.task_name,
        name: props.task.name,
        description: props.task.description || '',
        priority: props.task.priority,
        loop_interval: props.task.loop_interval || 60,
        cron_expression: props.task.cron_expression || '',
        is_active: props.task.is_active
      }
    } else {
      // 创建模式
      formData.value = {
        task_type: 'loop',
        task_name: '',
        name: '',
        description: '',
        priority: 'normal',
        loop_interval: 60,
        cron_expression: '',
        is_active: true
      }
    }
    errors.value = {}
  }
})

// 验证表单
function validateForm(): boolean {
  errors.value = {}
  
  if (!formData.value.name.trim()) {
    errors.value.name = t('validation.required')
  }
  
  if (!formData.value.task_name.trim()) {
    errors.value.task_name = t('validation.required')
  }
  
  if (formData.value.task_type === 'loop' && formData.value.loop_interval < 1) {
    errors.value.loop_interval = t('validation.minValue', { min: 1 })
  }
  
  if (formData.value.task_type === 'scheduled' && !formData.value.cron_expression.trim()) {
    errors.value.cron_expression = t('validation.required')
  }
  
  return Object.keys(errors.value).length === 0
}

// 提交表单
async function handleSubmit() {
  if (!validateForm()) {
    return
  }
  
  loading.value = true
  try {
    if (isEdit.value && props.task) {
      // 更新任务
      const updateData: UpdateTaskConfigRequest = {
        name: formData.value.name,
        description: formData.value.description || undefined,
        is_active: formData.value.is_active,
        priority: formData.value.priority
      }
      
      if (formData.value.task_type === 'loop') {
        updateData.loop_interval = formData.value.loop_interval
      } else if (formData.value.task_type === 'scheduled') {
        updateData.cron_expression = formData.value.cron_expression
      }
      
      await tasksService.updateTaskConfig(props.task.id, updateData)
      toast({
        title: t('common.success'),
        description: t('settings.tasks.updateSuccess')
      })
      emit('saved')
      emit('update:open', false)
    } else {
      // 创建任务
      const createData: CreateTaskConfigRequest = {
        task_type: formData.value.task_type,
        task_name: formData.value.task_name,
        name: formData.value.name,
        description: formData.value.description || undefined,
        priority: formData.value.priority
      }
      
      if (formData.value.task_type === 'loop') {
        createData.loop_interval = formData.value.loop_interval
      } else if (formData.value.task_type === 'scheduled') {
        createData.cron_expression = formData.value.cron_expression
      }
      
      await tasksService.createTaskConfig(createData)
      toast({
        title: t('common.success'),
        description: t('settings.tasks.createSuccess')
      })
      emit('saved')
      emit('update:open', false)
    }
  } catch (error: any) {
    toast({
      title: t('common.error'),
      description: error.message || t('settings.tasks.createFailed'),
      variant: 'error'
    })
  } finally {
    loading.value = false
  }
}

// 获取常见的 cron 表达式示例
const cronExamples = [
  { value: '0 * * * *', label: t('settings.tasks.cron.everyHour') },
  { value: '0 0 * * *', label: t('settings.tasks.cron.everyDay') },
  { value: '0 0 * * 1', label: t('settings.tasks.cron.everyMonday') },
  { value: '0 0 1 * *', label: t('settings.tasks.cron.everyMonth') }
]
</script>

<template>
  <Dialog :open="open" @update:open="$emit('update:open', $event)">
    <DialogContent class="sm:max-w-[500px]">
      <DialogHeader>
        <DialogTitle>{{ dialogTitle }}</DialogTitle>
        <DialogDescription>
          {{ isEdit ? t('settings.tasks.editDescription') : t('settings.tasks.createDescription') }}
        </DialogDescription>
      </DialogHeader>
      
      <div class="space-y-4 py-4">
        <!-- 任务名称 -->
        <div class="space-y-2">
          <Label for="name">{{ t('settings.tasks.name') }} *</Label>
          <Input
            id="name"
            v-model="formData.name"
            :placeholder="t('settings.tasks.namePlaceholder')"
            :class="{ 'border-destructive': errors.name }"
          />
          <p v-if="errors.name" class="text-sm text-destructive">{{ errors.name }}</p>
        </div>
        
        <!-- 任务描述 -->
        <div class="space-y-2">
          <Label for="description">{{ t('settings.tasks.taskDescription') }}</Label>
          <Textarea
            id="description"
            v-model="formData.description"
            :placeholder="t('settings.tasks.taskDescriptionPlaceholder')"
            rows="3"
          />
        </div>
        
        <!-- 任务类型（仅创建时可选） -->
        <div v-if="!isEdit" class="space-y-2">
          <Label for="task_type">{{ t('settings.tasks.type') }} *</Label>
          <Select v-model="formData.task_type">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="loop">{{ t('settings.tasks.types.loop') }}</SelectItem>
              <SelectItem value="scheduled">{{ t('settings.tasks.types.scheduled') }}</SelectItem>
              <SelectItem value="once">{{ t('settings.tasks.types.once') }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        
        <!-- 任务标识（仅创建时可选） -->
        <div v-if="!isEdit" class="space-y-2">
          <Label for="task_name">{{ t('settings.tasks.taskName') }} *</Label>
          <Input
            id="task_name"
            v-model="formData.task_name"
            :placeholder="t('settings.tasks.taskNamePlaceholder')"
            :class="{ 'border-destructive': errors.task_name }"
          />
          <p v-if="errors.task_name" class="text-sm text-destructive">{{ errors.task_name }}</p>
          <p class="text-sm text-muted-foreground">{{ t('settings.tasks.taskNameHelp') }}</p>
        </div>
        
        <!-- 优先级 -->
        <div class="space-y-2">
          <Label for="priority">{{ t('settings.tasks.taskPriority') }}</Label>
          <Select v-model="formData.priority">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="high">{{ t('settings.tasks.priorities.high') }}</SelectItem>
              <SelectItem value="normal">{{ t('settings.tasks.priorities.normal') }}</SelectItem>
              <SelectItem value="low">{{ t('settings.tasks.priorities.low') }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        
        <!-- 循环间隔（仅 loop 类型） -->
        <div v-if="formData.task_type === 'loop'" class="space-y-2">
          <Label for="loop_interval">{{ t('settings.tasks.loopInterval') }} *</Label>
          <div class="flex items-center space-x-2">
            <Input
              id="loop_interval"
              v-model.number="formData.loop_interval"
              type="number"
              min="1"
              :placeholder="t('settings.tasks.loopIntervalPlaceholder')"
              :class="{ 'border-destructive': errors.loop_interval }"
            />
            <span class="text-sm text-muted-foreground">{{ t('settings.tasks.seconds') }}</span>
          </div>
          <p v-if="errors.loop_interval" class="text-sm text-destructive">{{ errors.loop_interval }}</p>
        </div>
        
        <!-- Cron 表达式（仅 scheduled 类型） -->
        <div v-if="formData.task_type === 'scheduled'" class="space-y-2">
          <Label for="cron_expression">{{ t('settings.tasks.cronExpression') }} *</Label>
          <Input
            id="cron_expression"
            v-model="formData.cron_expression"
            :placeholder="t('settings.tasks.cronPlaceholder')"
            :class="{ 'border-destructive': errors.cron_expression }"
          />
          <p v-if="errors.cron_expression" class="text-sm text-destructive">{{ errors.cron_expression }}</p>
          <div class="space-y-1">
            <p class="text-sm text-muted-foreground">{{ t('settings.tasks.cronExamples') }}</p>
            <div class="text-sm text-muted-foreground">
              <div v-for="example in cronExamples" :key="example.value">
                <code class="bg-muted px-1">{{ example.value }}</code> - {{ example.label }}
              </div>
            </div>
          </div>
        </div>
      </div>
      
      <DialogFooter>
        <Button variant="outline" @click="$emit('update:open', false)" :disabled="loading">
          {{ t('common.cancel') }}
        </Button>
        <Button @click="handleSubmit" :disabled="loading">
          {{ loading ? t('common.saving') : t('common.save') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>