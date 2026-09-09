// Package privacy 管理阅读隐私相关的持久状态：目前只有「总是显示远程图片」的发件人信任名单。
package privacy

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TrustedSender 是允许自动加载远程图片的发件地址（小写、精确匹配）。
// 只按地址不按域名：「总是显示此发件人的图片」是对一个人的信任，域名级会把整家营销商放进来。
type TrustedSender struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Address   string    `gorm:"uniqueIndex;not null" json:"address"`
	CreatedAt time.Time `json:"created_at"`
}

func (TrustedSender) TableName() string { return "trusted_senders" }

var (
	ErrInvalid   = errors.New("invalid address")
	ErrDuplicate = errors.New("already trusted")
	ErrNotFound  = errors.New("not found")
)

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// Normalize 归一化地址：小写、去空白；必须形如 local@domain。
func Normalize(addr string) string {
	a := strings.ToLower(strings.TrimSpace(addr))
	at := strings.IndexByte(a, '@')
	if at <= 0 || at == len(a)-1 || strings.ContainsAny(a, " \t\r\n<>,;\"") || strings.Count(a, "@") != 1 {
		return ""
	}
	return a
}

func (s *Service) List() ([]TrustedSender, error) {
	var list []TrustedSender
	err := s.db.Order("id DESC").Find(&list).Error
	return list, err
}

func (s *Service) Add(addr string) (*TrustedSender, error) {
	a := Normalize(addr)
	if a == "" {
		return nil, fmt.Errorf("%w: %q", ErrInvalid, addr)
	}
	e := &TrustedSender{Address: a}
	if err := s.db.Create(e).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return e, nil
}

func (s *Service) Delete(id uint) error {
	res := s.db.Delete(&TrustedSender{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// IsTrusted 判断发件地址是否在信任名单里。每次打开邮件查一次，表很小，不缓存。
func (s *Service) IsTrusted(addr string) bool {
	a := Normalize(addr)
	if a == "" {
		return false
	}
	var n int64
	if err := s.db.Model(&TrustedSender{}).Where("address = ?", a).Count(&n).Error; err != nil {
		return false
	}
	return n > 0
}

// RegisterRoutes 挂载 /privacy/trusted-senders 增删查。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET("/privacy/trusted-senders", h.list)
	rg.POST("/privacy/trusted-senders", h.add)
	rg.DELETE("/privacy/trusted-senders/:id", h.remove)
}

type handler struct{ svc *Service }

func (h *handler) list(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"senders": list})
}

func (h *handler) add(c *gin.Context) {
	var body struct {
		Address string `json:"address"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	e, err := h.svc.Add(body.Address)
	switch {
	case errors.Is(err, ErrInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": "地址格式不正确"})
	case errors.Is(err, ErrDuplicate):
		c.JSON(http.StatusConflict, gin.H{"error": "already exists"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusCreated, e)
	}
}

func (h *handler) remove(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
