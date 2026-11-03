//go:build linux || darwin

package fsops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplacePublishesExclusivelyAndPreservesOldHold(t *testing.T) {
	for _, change := range []string{"none", "changed-target", "changed-source", "occupied-hold", "target-symlink"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			source, target, hold := filepath.Join(dir, "staged"), filepath.Join(dir, "tvshow.nfo"), filepath.Join(dir, "hold")
			for path, body := range map[string]string{source: "new nfo", target: "old nfo"} {
				if err := os.WriteFile(path, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			f, err := New([]string{dir})
			if err != nil {
				t.Fatal(err)
			}
			old, _ := os.Stat(target)
			staged, _ := os.Stat(source)
			switch change {
			case "changed-target":
				err = os.WriteFile(target, []byte("external target"), 0600)
			case "changed-source":
				err = os.Rename(source, source+".old")
				if err == nil {
					err = os.WriteFile(source, []byte("foreign stage"), 0600)
				}
			case "occupied-hold":
				err = os.WriteFile(hold, []byte("foreign hold"), 0600)
			case "target-symlink":
				err = os.Rename(target, target+".old")
				if err == nil {
					err = os.Symlink(target+".old", target)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			err = f.Replace(source, target, hold, Identity(old), old.Size(), old.ModTime().UnixNano(), Identity(staged))
			if change == "none" {
				if err != nil {
					t.Fatal(err)
				}
				for path, want := range map[string]string{target: "new nfo", hold: "old nfo"} {
					if got, e := os.ReadFile(path); e != nil || string(got) != want {
						t.Fatal(path, string(got), e)
					}
				}
				if _, err := os.Stat(source); !os.IsNotExist(err) {
					t.Fatal("staging source survived", err)
				}
			} else {
				if err == nil {
					t.Fatal("accepted changed file")
				}
				want := "old nfo"
				if change == "changed-target" {
					want = "external target"
				}
				if got, e := os.ReadFile(target); e != nil || string(got) != want {
					t.Fatal("target changed", string(got), e)
				}
				if change == "occupied-hold" {
					if got, _ := os.ReadFile(hold); string(got) != "foreign hold" {
						t.Fatal("hold overwritten")
					}
				}
			}
		})
	}
}
