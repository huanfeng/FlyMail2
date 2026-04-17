export interface EmailAddress {
  name: string
  email: string
}

export interface Email {
  id: number
  email_id: number
  from: EmailAddress
  to: EmailAddress[]
  cc?: EmailAddress[]
  bcc?: EmailAddress[]
  subject: string
  body: string
  textBody?: string
  body_html?: string
  createdAt: string
  date: string
  is_read: boolean
  is_starred: boolean
  attachments?: Attachment[]
}

export interface Attachment {
  attachment_id: number
  email_id: number
  filename: string
  content_type: string
  size: number
}