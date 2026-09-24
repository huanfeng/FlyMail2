package aiprovider

import (
	"errors"

	"gorm.io/gorm"
)

// ErrNotFound 表示配置不存在。
var ErrNotFound = errors.New("AI 配置不存在")

// ErrOrderMismatch 表示重排请求里的 ID 集合与库里的不一致（客户端手里的列表过时了）。
var ErrOrderMismatch = errors.New("配置列表已变化，请刷新后重新排序")

// Repository 负责 ai_providers 表的读写。
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Create 写入配置并排到末尾。
func (r *Repository) Create(p *Provider) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var max int
		if err := tx.Model(&Provider{}).Select("COALESCE(MAX(sort_order), 0)").Scan(&max).Error; err != nil {
			return err
		}
		p.SortOrder = max + 1
		return tx.Create(p).Error
	})
}

// List 按使用顺序返回全部配置（含停用的）。id 兜底，保证同位次时顺序稳定。
func (r *Repository) List() ([]Provider, error) {
	var list []Provider
	err := r.db.Order("sort_order asc, id asc").Find(&list).Error
	return list, err
}

func (r *Repository) Get(id uint) (*Provider, error) {
	var p Provider
	err := r.db.First(&p, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Update 只写可编辑的列。
//
// 不用 db.Save：Save 会把整行写回，调用方手里那份若是自己拼的（没读过库），
// CreatedAt 就会被零值覆盖（M13 草稿踩过这个坑）。
func (r *Repository) Update(p *Provider) error {
	res := r.db.Model(&Provider{ID: p.ID}).
		Select("name", "base_url", "api_key", "model", "enabled").
		Updates(p)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) Delete(id uint) error {
	res := r.db.Delete(&Provider{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Count 返回配置条数。
func (r *Repository) Count() (int64, error) {
	var n int64
	err := r.db.Model(&Provider{}).Count(&n).Error
	return n, err
}

// Reorder 按给定的完整 ID 顺序重写位次。
//
// 与账户排序同一个约定（见 account.Repository.Reorder）：接完整顺序而不是相对移动，
// 集合对不上就拒绝，不替用户猜漏掉的那条该放哪儿。
func (r *Repository) Reorder(ids []uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing []uint
		if err := tx.Model(&Provider{}).Pluck("id", &existing).Error; err != nil {
			return err
		}
		if !sameIDSet(existing, ids) {
			return ErrOrderMismatch
		}
		for i, id := range ids {
			if err := tx.Model(&Provider{}).Where("id = ?", id).
				Update("sort_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// sameIDSet 报告两个 ID 列表是否是同一个集合（忽略顺序，重复算不同）。
func sameIDSet(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[uint]int, len(a))
	for _, id := range a {
		seen[id]++
	}
	for _, id := range b {
		if seen[id] == 0 {
			return false
		}
		seen[id]--
	}
	return true
}
