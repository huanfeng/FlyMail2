<script lang="ts" setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { X } from 'lucide-vue-next'

const router = useRouter()

const to = ref('')
const cc = ref('')
const bcc = ref('')
const subject = ref('')
const content = ref('')

function handleSend() {
  // TODO: Implement send functionality
  console.log('Sending email:', { to: to.value, cc: cc.value, bcc: bcc.value, subject: subject.value, content: content.value })
}

function handleSaveDraft() {
  // TODO: Implement save draft functionality
  console.log('Saving draft')
}

function handleClose() {
  router.back()
}
</script>

<template>
  <div class="h-screen flex flex-col bg-background">
    <div class="flex items-center justify-between p-4 border-b">
      <h1 class="text-xl font-semibold">新邮件</h1>
      <Button variant="ghost" size="icon" @click="handleClose">
        <X class="h-4 w-4" />
      </Button>
    </div>
    
    <div class="flex-1 overflow-auto">
      <div class="p-4 space-y-4">
        <div class="space-y-2">
          <Label for="to">收件人</Label>
          <Input id="to" v-model="to" placeholder="输入收件人邮箱" />
        </div>
        
        <div class="space-y-2">
          <Label for="cc">抄送</Label>
          <Input id="cc" v-model="cc" placeholder="输入抄送邮箱" />
        </div>
        
        <div class="space-y-2">
          <Label for="bcc">密送</Label>
          <Input id="bcc" v-model="bcc" placeholder="输入密送邮箱" />
        </div>
        
        <div class="space-y-2">
          <Label for="subject">主题</Label>
          <Input id="subject" v-model="subject" placeholder="输入邮件主题" />
        </div>
        
        <div class="space-y-2">
          <Label for="content">正文</Label>
          <Textarea 
            id="content" 
            v-model="content" 
            placeholder="输入邮件内容" 
            class="min-h-[300px]"
          />
        </div>
      </div>
    </div>
    
    <div class="flex items-center justify-between p-4 border-t">
      <Button variant="outline" @click="handleSaveDraft">
        保存草稿
      </Button>
      <Button @click="handleSend">
        发送
      </Button>
    </div>
  </div>
</template>