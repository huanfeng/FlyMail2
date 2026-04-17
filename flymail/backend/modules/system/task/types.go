package task

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// Type represents task type
type Type string

const (
	TypeLoop      Type = "loop"      // 循环执行
	TypeScheduled Type = "scheduled" // 定时任务
	TypeOnce      Type = "once"      // 单次任务
)

// Priority represents task priority
type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityNormal Priority = "normal"
	PriorityLow    Priority = "low"
)

// Status represents task status
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// Predefined task names
const (
	NameEmailMonitor     = "email_monitor"
	NameEmailSyncHistory = "email_sync_history"
	NameSystemBackup     = "system_backup"
	NameDatabasePrune    = "database_prune"
)

// Config represents task configuration
type Config struct {
	ID          uint     `gorm:"primaryKey;column:id" json:"id"`
	UserID      uint     `gorm:"not null;default:1;index" json:"user_id"`
	TaskType    Type     `gorm:"not null;index" json:"task_type"`
	TaskName    string   `gorm:"not null;index" json:"task_name"`
	TaskID      string   `gorm:"uniqueIndex;not null" json:"task_id"` // task_name + 具体标识
	Name        string   `gorm:"not null" json:"name"`                // 显示名称
	Description string   `json:"description"`
	IsActive    bool     `gorm:"default:true;index" json:"is_active"`
	Priority    Priority `gorm:"not null;default:'normal'" json:"priority"`

	// 任务类型特定配置
	LoopInterval   int        `json:"loop_interval,omitempty"`   // 循环间隔(秒) - 用于 loop 类型
	CronExpression string     `json:"cron_expression,omitempty"` // Cron 表达式 - 用于 scheduled 类型
	ExecuteAt      *time.Time `json:"execute_at,omitempty"`      // 执行时间 - 用于 once 类型

	// 任务执行配置
	ExtraConfig json.RawMessage `gorm:"type:text" json:"extra_config,omitempty"` // 任务特定参数
	MaxRetry    int             `gorm:"default:3" json:"max_retry"`
	Timeout     int             `gorm:"default:300" json:"timeout"` // 超时时间(秒)

	// 执行状态跟踪
	IsRunning  bool       `gorm:"default:false;index" json:"is_running"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	LastStatus Status     `json:"last_status,omitempty"`
	NextRunAt  *time.Time `json:"next_run_at,omitempty"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName specifies the table name
func (Config) TableName() string {
	return "task_configs"
}

// Log represents task execution log
type Log struct {
	ID           uint            `gorm:"primaryKey;column:id" json:"id"`
	UserID       uint            `gorm:"not null;index" json:"user_id"`
	TaskType     Type            `gorm:"not null;index" json:"task_type"`
	TaskName     string          `gorm:"not null;index" json:"task_name"`
	TaskID       string          `gorm:"not null;index" json:"task_id"` // 冗余存储，不使用外键
	Status       Status          `gorm:"not null;index" json:"status"`
	StartTime    time.Time       `gorm:"not null;index" json:"start_time"`
	EndTime      *time.Time      `json:"end_time,omitempty"`
	Duration     int64           `json:"duration,omitempty"` // 执行时长(毫秒)
	ErrorMessage string          `gorm:"type:text" json:"error_message,omitempty"`
	Result       json.RawMessage `gorm:"type:text" json:"result,omitempty"` // 执行结果
	CreatedAt    time.Time       `json:"created_at"`
}

// TableName specifies the table name
func (Log) TableName() string {
	return "task_logs"
}

// Task represents a task to be executed
type Task struct {
	TaskType    Type
	TaskName    string
	TaskID      string
	ExtraConfig json.RawMessage
	Priority    Priority
	UserID      uint
	ConfigID    uint
}

// Event represents a task event
type Event struct {
	Type      string    `json:"type"`
	TaskID    string    `json:"task_id"`
	TaskName  string    `json:"task_name"`
	Status    Status    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Handler interface for task execution
type Handler interface {
	TaskName() string
	Execute(ctx context.Context, task *Task) error
}

// JSONMap represents a map that can be stored as JSON
type JSONMap map[string]interface{}

// Value implements driver.Valuer interface
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// Scan implements sql.Scanner interface
func (m *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data = []byte(value.(string))
	}

	return json.Unmarshal(data, m)
}
