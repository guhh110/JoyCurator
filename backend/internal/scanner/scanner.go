package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"joycurator/internal/fsops"
	"joycurator/internal/model"
)

var videoExt = map[string]bool{".mkv": true, ".mp4": true, ".avi": true, ".mov": true, ".m4v": true, ".ts": true, ".m2ts": true, ".wmv": true, ".webm": true, ".mpg": true, ".mpeg": true, ".flv": true}

func Probe(ctx context.Context, access *fsops.FS, path string) model.Probe {
	deadline, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	file, err := access.Open(path)
	if err != nil {
		return model.Probe{Warning: "媒体文件不可读"}
	}
	defer file.Close()
	cmd := exec.CommandContext(deadline, "ffprobe", "-v", "error", "-show_entries", "stream=codec_type,codec_name,width,height:format=duration", "-of", "json", "-i", "/dev/fd/3")
	cmd.ExtraFiles = []*os.File{file}
	bytes, err := cmd.Output()
	if err != nil {
		return model.Probe{Warning: "ffprobe 不可用或无法读取媒体；画质字段暂为空"}
	}
	var data struct {
		Streams []struct {
			Type   string `json:"codec_type"`
			Codec  string `json:"codec_name"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if json.Unmarshal(bytes, &data) != nil {
		return model.Probe{Warning: "媒体探测返回格式错误"}
	}
	p := model.Probe{}
	p.Duration, _ = strconv.ParseFloat(data.Format.Duration, 64)
	for _, stream := range data.Streams {
		if stream.Type == "video" {
			p.Codec = stream.Codec
			if stream.Width >= 3800 || stream.Height >= 2000 {
				p.Resolution = "2160p"
			} else if stream.Width >= 1900 || stream.Height >= 1000 {
				p.Resolution = "1080p"
			} else if stream.Width >= 1200 || stream.Height >= 700 {
				p.Resolution = "720p"
			} else if stream.Height > 0 {
				p.Resolution = fmt.Sprintf("%dp", stream.Height)
			}
			break
		}
	}
	return p
}
func IsVideo(name string) bool { return videoExt[strings.ToLower(filepath.Ext(name))] }

func Walk(ctx context.Context, access *fsops.FS, sources, excluded []string, visit func(string, fs.FileInfo) error) error {
	canonicalExcluded := make([]string, len(excluded))
	for i, path := range excluded {
		if path != "" {
			canonicalExcluded[i] = access.Canonical(path)
		}
	}
	excluded = canonicalExcluded
	seen := map[string]bool{}
	for _, source := range sources {
		source = access.Canonical(source)
		root, rel, err := access.Root(source)
		if err != nil {
			return err
		}
		err = fs.WalkDir(root.FS(), rel, func(name string, d fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				return walkErr
			}
			abs := filepath.Join(root.Name(), name)
			if d.Type()&fs.ModeSymlink != 0 {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				for _, e := range excluded {
					if e != "" && fsops.Within(e, abs) {
						return fs.SkipDir
					}
				}
				if strings.HasPrefix(d.Name(), ".") && abs != source {
					return fs.SkipDir
				}
				return nil
			}
			if seen[abs] || !videoExt[strings.ToLower(filepath.Ext(name))] || strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			seen[abs] = true
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			return visit(abs, info)
		})
		root.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
