package recognition

import (
	"errors"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode"
)

var idRE = regexp.MustCompile(`(?i)tmdb[-= ]?(\d+)`)

type pathInput struct {
	path, root, relative, filename string
	directories                    []string
}

func prepare(in Input) (pathInput, error) {
	if len(in.Path) == 0 || len(in.Path) > 16384 || strings.IndexFunc(in.Path, unicode.IsControl) >= 0 {
		return pathInput{}, errors.New("请输入完整的绝对文件路径")
	}

	// The built-in examples are POSIX paths even when the server runs on Windows.
	// Only drive/UNC paths use the host's Windows path semantics.
	posix := strings.HasPrefix(in.Path, "/")
	if !posix && (runtime.GOOS != "windows" || !filepath.IsAbs(in.Path)) {
		return pathInput{}, errors.New("请输入完整的绝对文件路径")
	}
	clean, isAbs, dir, base, ext := path.Clean, path.IsAbs, path.Dir, path.Base, path.Ext
	rootDefault := "/"
	if !posix {
		clean, isAbs, dir, base, ext = filepath.Clean, filepath.IsAbs, filepath.Dir, filepath.Base, filepath.Ext
		rootDefault = filepath.VolumeName(in.Path) + string(filepath.Separator)
	}
	if strings.HasSuffix(in.Path, "/") || (!posix && strings.HasSuffix(in.Path, `\`)) {
		return pathInput{}, errors.New("请输入文件路径，不能只提供目录")
	}
	file := clean(in.Path)
	if file == rootDefault {
		return pathInput{}, errors.New("请输入文件路径，不能只提供目录")
	}
	root := in.Root
	if root == "" {
		root = rootDefault
	}
	if strings.IndexFunc(root, unicode.IsControl) >= 0 || !isAbs(root) || (posix != strings.HasPrefix(root, "/")) {
		return pathInput{}, errors.New("解析边界必须是绝对目录路径")
	}
	root = clean(root)
	var rel string
	if posix {
		prefix := strings.TrimSuffix(root, "/") + "/"
		if !strings.HasPrefix(file, prefix) {
			return pathInput{}, errors.New("解析边界必须是文件路径的父级目录")
		}
		rel = strings.TrimPrefix(file, prefix)
	} else {
		var err error
		rel, err = filepath.Rel(root, file)
		if err != nil || rel == "." || !filepath.IsLocal(rel) {
			return pathInput{}, errors.New("解析边界必须是文件路径的父级目录")
		}
		rel = filepath.ToSlash(rel)
	}
	filename := strings.TrimSuffix(base(file), ext(file))
	p := pathInput{path: file, root: root, relative: rel, filename: filename, directories: []string{}}
	for parent := dir(file); parent != root && parent != dir(parent); parent = dir(parent) {
		p.directories = append(p.directories, base(parent))
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
		return []string{p.relative}
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
