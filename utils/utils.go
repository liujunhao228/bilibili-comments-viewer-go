// utils/utils.go
package utils

import (
	"os"
	"path/filepath"
	"strconv"
)

// 确保目录存在
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// 字符串转整数
func StringToInt(s string) (int, error) {
	return strconv.Atoi(s)
}

// 读取cookie文件
func ReadCookie(cookieFile string) string {
	if data, err := os.ReadFile(cookieFile); err == nil {
		return string(data)
	}
	return ""
}

// 获取绝对路径
func AbsPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}