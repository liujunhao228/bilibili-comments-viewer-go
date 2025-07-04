package util

import "strings"

func SanitizeFileName(title string) string {
	for _, c := range []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"} {
		title = strings.ReplaceAll(title, c, "_")
	}
	return title
}