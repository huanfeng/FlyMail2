package rule

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrNotFound  = errors.New("rule not found")
	ErrDuplicate = errors.New("block entry already exists")
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// ── 规则 ──────────────────────────────────────────────────────────────────────

func (r *Repository) List() ([]Rule, error) {
	var rules []Rule
	err := r.db.Order("priority ASC").Order("id ASC").Find(&rules).Error
	return rules, err
}

// ListEnabledFor 返回对某账户生效（account_id = 0 或等于该账户）的启用规则，按优先级升序。
func (r *Repository) ListEnabledFor(accountID uint) ([]Rule, error) {
	var rules []Rule
	err := r.db.Where("enabled = ? AND (account_id = 0 OR account_id = ?)", true, accountID).
		Order("priority ASC").Order("id ASC").Find(&rules).Error
	return rules, err
}

func (r *Repository) Get(id uint) (*Rule, error) {
	var rule Rule
	err := r.db.First(&rule, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &rule, err
}

// Create 新规则排到最后（priority = 当前最大 + 1）。
func (r *Repository) Create(rule *Rule) error {
	var maxPrio *int
	if err := r.db.Model(&Rule{}).Select("MAX(priority)").Scan(&maxPrio).Error; err != nil {
		return err
	}
	if maxPrio != nil {
		rule.Priority = *maxPrio + 1
	}
	return r.db.Create(rule).Error
}

func (r *Repository) Update(rule *Rule) error {
	res := r.db.Model(&Rule{}).Where("id = ?", rule.ID).Updates(map[string]any{
		"name": rule.Name, "enabled": rule.Enabled, "account_id": rule.AccountID, "match": rule.Match,
		"conditions": rule.Conditions, "actions": rule.Actions, "stop_processing": rule.StopProcessing,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) SetEnabled(id uint, enabled bool) error {
	return r.db.Model(&Rule{}).Where("id = ?", id).Update("enabled", enabled).Error
}

func (r *Repository) Delete(id uint) error {
	res := r.db.Delete(&Rule{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Reorder 按 ids 的顺序重写 priority（0..n-1）；未列出的规则排在其后、保持原相对顺序。
// ids 里有不存在的规则返回 ErrInvalid（多半是前端缓存过期，静默接受会让顺序悄悄错位）。
func (r *Repository) Reorder(ids []uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var all []Rule
		if err := tx.Order("priority ASC").Order("id ASC").Find(&all).Error; err != nil {
			return err
		}
		exists := make(map[uint]bool, len(all))
		for _, rule := range all {
			exists[rule.ID] = true
		}
		pos := map[uint]int{}
		for i, id := range ids {
			if !exists[id] {
				return fmt.Errorf("%w: 规则 %d 不存在", ErrInvalid, id)
			}
			pos[id] = i
		}
		next := len(ids)
		for _, rule := range all {
			p, ok := pos[rule.ID]
			if !ok {
				p = next
				next++
			}
			if p != rule.Priority {
				if err := tx.Model(&Rule{}).Where("id = ?", rule.ID).Update("priority", p).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// ── 执行日志 ──────────────────────────────────────────────────────────────────

// RecordRuns 批量记执行日志（一条语句分批写）；(account, key, rule) 已存在的静默忽略（幂等）。
func (r *Repository) RecordRuns(runs []RuleRun) error {
	if len(runs) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(runs, 200).Error
}

// RanRules 返回一批邮件已被哪些规则处理过：message_key → {rule_id}。
func (r *Repository) RanRules(accountID uint, keys []string) (map[string]map[uint]bool, error) {
	out := map[string]map[uint]bool{}
	if len(keys) == 0 {
		return out, nil
	}
	var rows []RuleRun
	if err := r.db.Select("message_key, rule_id").
		Where("account_id = ? AND message_key IN ?", accountID, keys).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if out[row.MessageKey] == nil {
			out[row.MessageKey] = map[uint]bool{}
		}
		out[row.MessageKey][row.RuleID] = true
	}
	return out, nil
}

func (r *Repository) ListRuns(limit int) ([]RuleRun, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var runs []RuleRun
	err := r.db.Order("id DESC").Limit(limit).Find(&runs).Error
	return runs, err
}

// PruneRuns 清理早于 before 的执行日志（幂等只需要覆盖「同一封邮件再次入库」的窗口，几个月足够）。
func (r *Repository) PruneRuns(before time.Time) error {
	return r.db.Where("created_at < ?", before).Delete(&RuleRun{}).Error
}

// ── 黑名单 ────────────────────────────────────────────────────────────────────

func (r *Repository) ListBlocks() ([]BlockEntry, error) {
	var entries []BlockEntry
	err := r.db.Order("id DESC").Find(&entries).Error
	return entries, err
}

// BlockPatterns 只取模式列，规则执行每轮都要读。
func (r *Repository) BlockPatterns() ([]string, error) {
	var patterns []string
	err := r.db.Model(&BlockEntry{}).Order("id ASC").Pluck("pattern", &patterns).Error
	return patterns, err
}

// AddBlock 依赖 pattern 的唯一索引判重：并发插入同一条时靠约束兜底，冲突统一转成 ErrDuplicate。
func (r *Repository) AddBlock(e *BlockEntry) error {
	err := r.db.Create(e).Error
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return ErrDuplicate
	}
	return err
}

func (r *Repository) DeleteBlock(id uint) error {
	res := r.db.Delete(&BlockEntry{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
