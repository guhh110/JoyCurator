package recognition

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var idRE = regexp.MustCompile(`(?i)tmdb[-= ]?(\d+)`)

type pathInput struct {
	path, root, filename string
	directories          []string
}

func prepare(in Input) (pathInput, error) {
	if len(in.Path) == 0 || len(in.Path) > 16384 || !strings.HasPrefix(in.Path, "/") || strings.IndexFunc(in.Path, unicode.IsControl) >= 0 {
		return pathInput{}, errors.New("请输入以 / 开头的完整文件路径")
	}
	path := filepath.Clean(in.Path)
	if path == "/" || strings.HasSuffix(in.Path, "/") {
		return pathInput{}, errors.New("请输入文件路径，不能只提供目录")
	}
	root := in.Root
	if root == "" {
		root = "/"
	}
	if !filepath.IsAbs(root) || strings.IndexFunc(root, unicode.IsControl) >= 0 {
		return pathInput{}, errors.New("解析边界必须是绝对目录路径")
	}
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(rel) || root == path {
		return pathInput{}, errors.New("解析边界必须是文件路径的父级目录")
	}
	filename := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	p := pathInput{path: path, root: root, filename: filename, directories: []string{}}
	for dir := filepath.Dir(path); dir != root && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		p.directories = append(p.directories, filepath.Base(dir))
	}
	return p, nil
}
func (p pathInput) sources(source Source) []string {
	switch source {
	case Filename:
		return []string{p.filename}
	case CleanFilename:
		return []string{p.filename}
	case ParentDirectory:
		if len(p.directories) > 0 {
			return p.directories[:1]
		}
	case Directories:
		return p.directories
	case RelativePath:
		rel, _ := filepath.Rel(p.root, p.path)
		return []string{rel}
	case AbsolutePath:
		return []string{p.path}
	}
	return []string{}
}
func NormalizeTitle(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
func protected(text string, start, end int) bool {
	for _, span := range idRE.FindAllStringIndex(text, -1) {
		if start < span[1] && end > span[0] {
			return true
		}
	}
	return false
}
