package util

import (
	"bufio"
	"os"
	"strings"
)

func LoadCookies(cookieFile string) string {
	file, err := os.Open(cookieFile)
	if err != nil {
		return ""
	}
	defer file.Close()
	
	var sb strings.Builder
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			sb.WriteString(line + "; ")
		}
	}
	return strings.TrimSuffix(sb.String(), "; ")
}

func CreateDirIfNotExist(dir string) error {
	return os.MkdirAll(dir, 0755)
}