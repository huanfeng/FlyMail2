package core

import (
	"mail2im/internal/models"
)

func GetSystemSetting(key string) (string, error) {
	var setting models.SystemSetting
	if err := DB.Where("key = ?", key).First(&setting).Error; err != nil {
		return "", err
	}
	return setting.Value, nil
}

func SetSystemSetting(key, value string) error {
	var setting models.SystemSetting
	return DB.Where(models.SystemSetting{Key: key}).
		Assign(models.SystemSetting{Value: value}).
		FirstOrCreate(&setting).Error
}

func GetSystemSettingWithDefault(key, defaultValue string) string {
	val, err := GetSystemSetting(key)
	if err != nil {
		return defaultValue
	}
	return val
}
