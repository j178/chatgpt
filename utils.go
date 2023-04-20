package chatgpt

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func EnsureTrailingNewline(s string) string {
	if !strings.HasSuffix(s, "\n") {
		return s + "\n"
	}
	return s
}

func CreateIfNotExists(path string, isDir bool) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if isDir {
				return os.MkdirAll(path, 0o755)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o666); err != nil {
				return err
			}
			f, err := os.OpenFile(path, os.O_CREATE, 0o666)
			if err != nil {
				return err
			}
			_ = f.Close()
		}
	}
	return nil
}

func ContainsCJK(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana) {
			return true
		}
	}
	return false
}
