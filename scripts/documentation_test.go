package scripts

import (
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Public documentation should remain navigable from a fresh checkout.
func TestDocumentationLocalLinks(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	links := regexp.MustCompile(`\[[^\]\n]*\]\(([^)\n]+)\)`)
	images := regexp.MustCompile(`<img\b[^>]*\bsrc="([^"]+)"`)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "bin", "node_modules", "runs", "evidence", "evidence-v0.1.1":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		references := links.FindAllSubmatch(data, -1)
		references = append(references, images.FindAllSubmatch(data, -1)...)
		for _, match := range references {
			target := strings.Trim(string(match[1]), "<>")
			u, err := url.Parse(target)
			if err != nil {
				t.Errorf("%s: invalid link %q: %v", path, target, err)
				continue
			}
			if u.IsAbs() || u.Host != "" {
				continue
			}
			destination := path
			if u.Path != "" {
				destination = filepath.Join(filepath.Dir(path), filepath.FromSlash(u.Path))
			}
			info, err := os.Stat(destination)
			if err != nil {
				t.Errorf("%s: broken link %q: %v", path, target, err)
				continue
			}
			if u.Fragment == "" || info.IsDir() || filepath.Ext(destination) != ".md" {
				continue
			}
			headings, err := os.ReadFile(destination)
			if err != nil {
				return err
			}
			if !markdownHasAnchor(string(headings), u.Fragment) {
				t.Errorf("%s: missing heading for %q", path, target)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func markdownHasAnchor(markdown, anchor string) bool {
	punctuation := regexp.MustCompile(`[^\pL\pN_ -]`)
	headings := regexp.MustCompile(`(?m)^#{1,6} +(.+)$`)
	seen := map[string]int{}
	for _, heading := range headings.FindAllStringSubmatch(markdown, -1) {
		slug := punctuation.ReplaceAllString(strings.ToLower(strings.TrimSpace(heading[1])), "")
		slug = strings.ReplaceAll(slug, " ", "-")
		index := seen[slug]
		seen[slug]++
		if index > 0 {
			slug += "-" + strconv.Itoa(index)
		}
		if slug == anchor {
			return true
		}
	}
	return false
}
