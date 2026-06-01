package setting

import "strconv"

// Service 封装设置业务逻辑。
type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

// GetInt 取整数设置；取不到或解析失败返回 def。
func (s *Service) GetInt(key string, def int) int {
	val, found, err := s.repo.Get(key)
	if err != nil || !found {
		return def
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return n
}

// SetMany 批量保存键值对。
func (s *Service) SetMany(m map[string]string) error {
	for k, v := range m {
		if err := s.repo.Set(k, v); err != nil {
			return err
		}
	}
	return nil
}

// All 返回所有已存储的设置；调用方负责补充默认值。
func (s *Service) All() map[string]string {
	m, err := s.repo.All()
	if err != nil {
		return map[string]string{}
	}
	return m
}
