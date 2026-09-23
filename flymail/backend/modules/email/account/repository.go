package account

import (
	"errors"

	"gorm.io/gorm"
)

var ErrAccountNotFound = errors.New("account not found")

// ErrOrderMismatch 表示提交的排序列表与库中账户集合对不上（多、少或有重复）。
var ErrOrderMismatch = errors.New("排序列表与当前账户不一致")

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Create 写入账户，并把它排到列表末尾。
//
// 位次在这一层分配而不是各个 Service 方法里：建账户有三条路径（手填、OAuth 建号、
// 配置导入），放在上面就得抄三遍，将来多一条还会漏。
func (r *Repository) Create(a *Account) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if a.SortOrder == 0 {
			next, err := nextSortOrder(tx)
			if err != nil {
				return err
			}
			a.SortOrder = next
		}
		return tx.Create(a).Error
	})
}

// nextSortOrder 返回末尾之后的位次。
//
// 用 COALESCE 而不是先查计数：空表时 MAX 返回 NULL，直接扫进 int 会报错。
// 从 1 起而不是 0，是为了跟「老库刚加列时全是 0」区分开——那批行靠 List 的
// id 兜底排序，新建的账户必须排在它们后面而不是混进去。
func nextSortOrder(tx *gorm.DB) (int, error) {
	var max int
	err := tx.Model(&Account{}).Select("COALESCE(MAX(sort_order), 0)").Scan(&max).Error
	return max + 1, err
}

func (r *Repository) GetByID(id uint) (*Account, error) {
	var a Account
	err := r.db.First(&a, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// List 按用户设定的顺序返回全部账户。
//
// id 是兜底而非装饰：老库刚加 sort_order 列时全表都是 0，只按 sort_order 排
// 在 SQLite 下顺序是不确定的，界面会每次刷新都换一个排法。
func (r *Repository) List() ([]Account, error) {
	var list []Account
	err := r.db.Order("sort_order asc, id asc").Find(&list).Error
	return list, err
}

// Reorder 按给定的 ID 顺序重写全表位次。
//
// 接的是**完整顺序**而不是「把 X 上移一位」这类相对操作：相对操作要求客户端
// 与服务端对当前顺序的认知完全一致，一旦不一致（另一个标签页刚加了账户）
// 就会错位且无从察觉。整份列表则是幂等的，写完必然是 0..n-1，没有重复也没有空洞。
//
// ids 必须与库中账户集合完全一致，否则返回 ErrOrderMismatch：少一个就意味着
// 调用方是照着一份过时的列表算出来的顺序，此时替用户猜剩下那个该放哪儿，
// 比让他重来一次更糟。
func (r *Repository) Reorder(ids []uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing []uint
		if err := tx.Model(&Account{}).Pluck("id", &existing).Error; err != nil {
			return err
		}
		if !sameIDSet(existing, ids) {
			return ErrOrderMismatch
		}
		for i, id := range ids {
			if err := tx.Model(&Account{}).Where("id = ?", id).
				Update("sort_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// sameIDSet 报告两个 ID 列表是否是同一个集合（忽略顺序，但重复算不同）。
func sameIDSet(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[uint]struct{}, len(a))
	for _, id := range a {
		set[id] = struct{}{}
	}
	for _, id := range b {
		if _, ok := set[id]; !ok {
			return false
		}
		// 删掉以拒绝重复 ID：[1,1] 与 [1,2] 长度相同，不删就会被判为一致，
		// 结果是一个账户被写两次位次、另一个原地不动。
		delete(set, id)
	}
	return len(set) == 0
}

func (r *Repository) Update(a *Account) error { return r.db.Save(a).Error }

// UpdateFields 局部更新指定列。
//
// 令牌刷新与状态流转都走这里而不是 Update：整行 Save 会覆盖并发写入的其他字段，
// 并且会连带重写 CreatedAt。
func (r *Repository) UpdateFields(id uint, fields map[string]any) error {
	res := r.db.Model(&Account{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAccountNotFound
	}
	return nil
}

func (r *Repository) SetEnabled(id uint, enabled bool) error {
	res := r.db.Model(&Account{}).Where("id = ?", id).Update("enabled", enabled)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAccountNotFound
	}
	return nil
}

func (r *Repository) IsEnabled(id uint) (bool, error) {
	var a Account
	err := r.db.Select("id", "enabled").First(&a, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, ErrAccountNotFound
	}
	if err != nil {
		return false, err
	}
	return a.Enabled, nil
}

// ListEnabledIDs 返回所有 enabled=true 账户的 ID。
func (r *Repository) ListEnabledIDs() ([]uint, error) {
	var ids []uint
	err := r.db.Model(&Account{}).Where("enabled = ?", true).Pluck("id", &ids).Error
	return ids, err
}

// Delete 删除账户，并在同一事务内清理它名下的全部数据。
//
// ⚠ 别只删 accounts 行：邮件、文件夹、正文、附件、草稿、规则、回写队列、通知
// 全都挂在 account_id 上，不一起删就成了界面上看不见、却一直占着空间的孤儿
// （2026-09-16 实测：删一个账户留下 1579 封邮件 + 14 个文件夹）。
// 表清单与删除顺序见 purge.go。
func (r *Repository) Delete(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Delete(&Account{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrAccountNotFound
		}
		return purgeAccountData(tx, id)
	})
}
