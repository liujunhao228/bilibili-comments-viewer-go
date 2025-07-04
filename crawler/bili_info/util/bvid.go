package util

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// 正则表达式：匹配BVID
var bvidPattern = regexp.MustCompile(`^BV[0-9A-Za-z]{10}$`)

// IsValidBVID 检查BV号是否有效
func IsValidBVID(bvid string) bool {
	return bvidPattern.MatchString(bvid)
}

// ReadBVIdsFromFile 从文件读取BV号列表
func ReadBVIdsFromFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var bvids []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			bvids = append(bvids, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return bvids, nil
}