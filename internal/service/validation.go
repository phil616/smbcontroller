package service

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"

	"smb-controller/internal/models"
)

var (
	validUsername  = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)
	validShareName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
)

func validateUsername(username string) error {
	if !validUsername.MatchString(username) {
		return errors.New("用户名格式不正确。建议：使用 3-32 位英文字母、数字或下划线，例如 smb_user01。解决方案：删除空格、中文、连字符或其他特殊字符后重试")
	}
	return nil
}

func ValidatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("密码强度不足。建议：至少 8 位，并包含大写字母、小写字母和数字。解决方案：例如使用 Admin123 或更强的密码")
	}
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	hasDigit := regexp.MustCompile(`[0-9]`).MatchString(password)
	if !hasUpper || !hasLower || !hasDigit {
		return errors.New("密码强度不足。建议：同时包含大写字母、小写字母和数字。解决方案：在当前密码中加入缺少的字符类型，例如 A、a、1")
	}
	return nil
}

func validateShareName(name string) error {
	if !validShareName.MatchString(name) {
		return errors.New("共享名格式不正确。建议：使用 1-64 位英文字母、数字、下划线或连字符，例如 DataFolder。解决方案：删除空格、中文或其他特殊字符后重试")
	}
	return nil
}

func validatePath(path string) error {
	if err := validateConfigValue(path, "path"); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return errors.New("路径必须是绝对路径。建议：以 / 开头，例如 /srv/samba/data。解决方案：不要使用 data 或 ./data 这类相对路径")
	}
	if filepath.Clean(path) != path {
		return errors.New("路径必须是规范路径。建议：使用 " + filepath.Clean(path) + "。解决方案：删除末尾多余斜杠、重复斜杠或 .. 片段，例如将 /srv/samba/ 改为 /srv/samba")
	}
	return nil
}

func validateConfigValue(value, field string) error {
	if strings.ContainsAny(value, "\r\n\x00") {
		return errors.New(field + " 不能包含换行或控制字符。建议：只填写单行文本。解决方案：删除换行后重试")
	}
	return nil
}

func validateAccess(access string) error {
	switch access {
	case models.AccessRead, models.AccessReadWrite, models.AccessNone:
		return nil
	default:
		return errors.New("权限值不正确。建议：选择无权限、只读或读写。解决方案：刷新页面后重新选择")
	}
}
