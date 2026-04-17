package core

type NotificationChannel interface {
	Name() string
	Send(event Event) error
	MinPriority() EventPriority
}

// DetailedSender is an optional interface to expose request/response details for logging.
type DetailedSender interface {
	SendWithDetails(event Event) (request string, response string, err error)
}

// TemplateAwareChannel can receive pre-rendered text instead of raw events.
type TemplateAwareChannel interface {
	NotificationChannel
	// SendRendered sends a pre-rendered message. The channel may wrap it
	// in its own formatting (e.g., Discord embed).
	SendRendered(rendered string, event Event) error
}

// DetailedTemplateChannel combines TemplateAwareChannel with request/response logging.
type DetailedTemplateChannel interface {
	TemplateAwareChannel
	SendRenderedWithDetails(rendered string, event Event) (request string, response string, err error)
}
