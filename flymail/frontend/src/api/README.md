# FlyMailPlus API Service Layer

This directory contains the API service layer for frontend-backend communication, based on the OpenAPI specification.

## Structure

```
src/api/
├── types.ts          # TypeScript type definitions from OpenAPI schemas
├── config.ts         # API configuration and endpoints
├── axios.ts          # Axios instance with interceptors
├── services/         # Individual service modules
│   ├── auth.ts       # Authentication service
│   ├── accounts.ts   # Email accounts management
│   ├── folders.ts    # Folders management
│   ├── emails.ts     # Email operations
│   ├── tasks.ts      # Async tasks management
│   ├── monitoring.ts # System monitoring
│   ├── settings.ts   # Settings and scheduled tasks
│   └── sse.ts        # Server-Sent Events for real-time updates
└── index.ts          # Main export file
```

## Usage Examples

### Authentication

```typescript
import { authService } from '@/api'

// Login
try {
  const result = await authService.login({
    username: 'admin',
    password: 'admin123'
  })
  console.log('Logged in:', result.user)
} catch (error) {
  console.error('Login failed:', error.message)
}

// Get current user
const user = await authService.getCurrentUser()

// Logout
authService.logout()
```

### Email Accounts

```typescript
import { accountsService } from '@/api'

// Get all accounts
const { accounts } = await accountsService.getAccounts()

// Create new account
const newAccount = await accountsService.createAccount({
  name: 'My Gmail',
  email: 'user@gmail.com',
  type: 'imap',
  imap_server: 'imap.gmail.com',
  imap_port: 993,
  imap_ssl: true,
  smtp_server: 'smtp.gmail.com',
  smtp_port: 587,
  smtp_ssl: true,
  username: 'user@gmail.com',
  password: 'app-specific-password'
})

// Test account connection
const testResult = await accountsService.testAccount(accountId)

// Sync account emails
const syncTask = await accountsService.syncAccount(accountId, {
  limit: 100,
  force: false
})
```

### Folders Management

```typescript
import { foldersService } from '@/api'

// Get folders for account
const folders = await foldersService.getFolders(accountId)

// Build folder tree for UI
const folderTree = foldersService.buildFolderTree(folders)

// Sync folders from server
const syncResult = await foldersService.syncFolders(accountId)
```

### Email Operations

```typescript
import { emailsService } from '@/api'

// Get emails with pagination
const emails = await emailsService.getEmails({
  page: 1,
  limit: 50,
  account_id: accountId,
  is_read: false
})

// Get email detail
const email = await emailsService.getEmail(emailId)

// Update email flags
await emailsService.markAsRead(emailId)
await emailsService.toggleStar(emailId, true)

// Batch operations
const result = await emailsService.batchUpdateFlags(
  [id1, id2, id3],
  { is_read: true }
)
```

### Real-time Updates (SSE)

```typescript
import { sseService } from '@/api'

// Connect to SSE with event handlers
sseService.connect({
  onConnected: (data) => {
    console.log('SSE connected:', data)
  },
  onNewEmails: (data) => {
    console.log(`New emails for account ${data.account_id}:`, data.emails)
  },
  onTaskProgress: (data) => {
    console.log(`Task ${data.task_id} progress: ${data.progress}%`)
  },
  onTaskCompleted: (task) => {
    console.log(`Task ${task.task_id} completed`)
  },
  onError: (error) => {
    console.error('SSE error:', error)
  }
})

// Disconnect when done
sseService.disconnect()
```

### Task Management

```typescript
import { tasksService } from '@/api'

// Get task list
const tasks = await tasksService.getTasks({
  type: 'email_sync',
  page: 1
})

// Monitor task progress
const finalTask = await tasksService.pollTaskStatus(
  taskId,
  (task) => {
    console.log(`Progress: ${task.progress}%`)
  }
)

// Cancel task
await tasksService.cancelTask(taskId)
```

### Settings (Admin Only)

```typescript
import { settingsService } from '@/api'

// Get app settings
const appSettings = await settingsService.getAppSettings()

// Update settings
await settingsService.updateAppSettings({
  email_sync_interval: 60,
  theme: 'dark'
})

// Manage scheduled tasks
const tasks = await settingsService.getScheduledTasks()
const newTask = await settingsService.createScheduledTask({
  name: 'Daily Sync',
  type: 'email_sync',
  schedule: '0 0 * * *',
  is_active: true
})
```

## Error Handling

All API methods throw errors with descriptive messages when requests fail. The axios interceptor handles:

- Automatic token refresh on 401 errors
- Network errors
- Server errors with proper error messages

```typescript
try {
  const result = await api.someMethod()
} catch (error) {
  // Error will contain a user-friendly message
  console.error(error.message)
}
```

## Authentication Flow

1. User logs in with `authService.login()`
2. Access token and refresh token are stored in localStorage
3. Access token is automatically added to all requests
4. When access token expires, refresh token is used to get new tokens
5. If refresh fails, user is logged out automatically

## Environment Variables

Configure the API base URL in `.env`:

```env
VITE_API_BASE_URL=http://localhost:8080/api/v1
```

## TypeScript Support

All API methods are fully typed based on the OpenAPI specification. This provides:

- Autocomplete for request parameters
- Type checking for responses
- Clear documentation through JSDoc comments