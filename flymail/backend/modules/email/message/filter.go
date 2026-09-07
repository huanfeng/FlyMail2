package message

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Filter 是列表接口的可叠加筛选条件（对应前端「未读 / 星标 / 有附件」三个独立开关）。
//
// 每个维度用指针表达三态：nil = 该维度不参与筛选，非 nil = 按该值筛选。
// 不能用 bool 零值表达「不筛选」——false 本身就是有意义的取值：
// seen=false 正是「只看未读」，与「不按已读状态筛选」是两回事。
//
// ⚠ HasAttachment 的准确性受正文预取进度限制：has_attachment 由 MarkBodySynced
// 在正文落库时回填，尚未预取正文的邮件该列恒为 false。因此「有附件」筛选
// 只在已预取正文的范围内可信，可能漏掉新到且未预取的带附件邮件。
type Filter struct {
	Seen          *bool
	Flagged       *bool
	HasAttachment *bool
}

// Active 表示是否有任一维度参与筛选。
// 列表接口据此决定是否要额外算一次筛选后的总数（不筛选时沿用 folders 表的现成计数）。
func (f Filter) Active() bool {
	return f.Seen != nil || f.Flagged != nil || f.HasAttachment != nil
}

// apply 把筛选条件附加到查询上。
//
// 列名一律带 messages. 前缀：聚合链路 JOIN 了 folders、搜索链路 JOIN 了 message_bodies，
// 裸列名在这些查询里有歧义（SQLite 会直接报 ambiguous column name）。
//
// seen / flagged 走部分索引 idx_msg_unread / idx_msg_flagged（见 model.go）：SQLite 会按绑定后的参数值
// 重新规划，`seen = ?` 绑 0 时同样选中部分索引（真实库 EXPLAIN QUERY PLAN 验证过），不必拼字面量。
func (f Filter) apply(q *gorm.DB) *gorm.DB {
	if f.Seen != nil {
		q = q.Where("messages.seen = ?", *f.Seen)
	}
	if f.Flagged != nil {
		q = q.Where("messages.flagged = ?", *f.Flagged)
	}
	if f.HasAttachment != nil {
		q = q.Where("messages.has_attachment = ?", *f.HasAttachment)
	}
	return q
}

// parseFilter 从查询串解析筛选条件：?seen=false&flagged=true&has_attachment=true
// 无法解析为布尔的取值（含空串）视为该维度不筛选，不报错——
// 筛选是渐进增强，一个拼错的参数不该让整个列表 400。
func parseFilter(c *gin.Context) Filter {
	return Filter{
		Seen:          parseTriBool(c.Query("seen")),
		Flagged:       parseTriBool(c.Query("flagged")),
		HasAttachment: parseTriBool(c.Query("has_attachment")),
	}
}

// parseTriBool 把查询串取值解析为三态布尔：空串/非法值 → nil。
func parseTriBool(s string) *bool {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		return nil
	}
	return &v
}
