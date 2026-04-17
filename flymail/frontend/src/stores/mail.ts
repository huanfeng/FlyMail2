import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export interface Mail {
  id: string
  name: string
  email: string
  subject: string
  text: string
  date: string
  read: boolean
  labels: string[]
  accountId: string
}

export interface Account {
  id: string
  label: string
  email: string
  icon: string
  folders: Folder[]
}

export interface Folder {
  id: string
  name: string
  icon: string
  count?: number
  children?: Folder[]
  sort_order?: number
}

export const useMailStore = defineStore('mail', () => {
  const accounts = ref<Account[]>([])
  const mails = ref<Mail[]>([])
  const selectedAccountId = ref<string>('')
  const selectedFolderId = ref<string>('inbox')
  const selectedMailId = ref<string>('')
  
  const currentAccount = computed(() => 
    accounts.value.find(acc => acc.id === selectedAccountId.value)
  )
  
  const currentMail = computed(() => 
    mails.value.find(mail => mail.id === selectedMailId.value)
  )
  
  const unreadMails = computed(() => 
    mails.value.filter(mail => !mail.read)
  )
  
  const starredMails = computed(() => 
    mails.value.filter(mail => mail.labels.includes('starred'))
  )
  
  const draftMails = computed(() => 
    mails.value.filter(mail => mail.labels.includes('draft'))
  )
  
  // Virtual folders for all accounts
  const allInboxMails = computed(() => 
    mails.value.filter(mail => mail.labels.includes('inbox'))
  )
  
  const allUnreadMails = computed(() => 
    mails.value.filter(mail => !mail.read)
  )
  
  const allStarredMails = computed(() => 
    mails.value.filter(mail => mail.labels.includes('starred'))
  )
  
  const allDraftMails = computed(() => 
    mails.value.filter(mail => mail.labels.includes('draft'))
  )
  
  function addAccount(account: Account) {
    accounts.value.push(account)
  }
  
  function removeAccount(accountId: string) {
    accounts.value = accounts.value.filter(acc => acc.id !== accountId)
    mails.value = mails.value.filter(mail => mail.accountId !== accountId)
  }
  
  function selectAccount(accountId: string) {
    selectedAccountId.value = accountId
  }
  
  function selectFolder(folderId: string) {
    selectedFolderId.value = folderId
  }
  
  function selectMail(mailId: string) {
    selectedMailId.value = mailId
    const mail = mails.value.find(m => m.id === mailId)
    if (mail) {
      mail.read = true
    }
  }
  
  function markAsRead(mailId: string) {
    const mail = mails.value.find(m => m.id === mailId)
    if (mail) {
      mail.read = true
    }
  }
  
  function markAsUnread(mailId: string) {
    const mail = mails.value.find(m => m.id === mailId)
    if (mail) {
      mail.read = false
    }
  }
  
  function toggleStar(mailId: string) {
    const mail = mails.value.find(m => m.id === mailId)
    if (mail) {
      const index = mail.labels.indexOf('starred')
      if (index > -1) {
        mail.labels.splice(index, 1)
      } else {
        mail.labels.push('starred')
      }
    }
  }
  
  return {
    accounts,
    mails,
    selectedAccountId,
    selectedFolderId,
    selectedMailId,
    currentAccount,
    currentMail,
    unreadMails,
    starredMails,
    draftMails,
    allInboxMails,
    allUnreadMails,
    allStarredMails,
    allDraftMails,
    addAccount,
    removeAccount,
    selectAccount,
    selectFolder,
    selectMail,
    markAsRead,
    markAsUnread,
    toggleStar,
  }
})