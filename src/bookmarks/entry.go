package bookmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

type BookmarkFile struct {
	Checksum     string                   `json:"checksum"`
	Roots        map[string]*BookmarkNode `json:"roots"`
	SyncMetadata string                   `json:"sync_metadata,omitempty"`
	Version      int                      `json:"version"`
}

type BookmarkNode struct {
	Children     []*BookmarkNode `json:"children,omitempty"`
	DateAdded    string          `json:"date_added"`
	DateLastUsed string          `json:"date_last_used,omitempty"`
	DateModified string          `json:"date_modified,omitempty"`
	GUID         string          `json:"guid,omitempty"`
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	URL          string          `json:"url,omitempty"`
}

func Path(user string) string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", user, "Bookmarks")
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data", user, "Bookmarks")
	default:
		return filepath.Join(home, ".config", "google-chrome", user, "Bookmarks")
	}
}

func Read(user string) (*BookmarkFile, error) {
	data, err := os.ReadFile(Path(user))
	if err != nil {
		return nil, err
	}
	var bf BookmarkFile
	return &bf, json.Unmarshal(data, &bf)
}

func Write(user string, bf *BookmarkFile) error {
	data, err := json.MarshalIndent(bf, "", "   ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(user), data, 0644)
}

func Add(bf *BookmarkFile, name, url string) {
	bar := bf.Roots["bookmark_bar"]
	ts := fmt.Sprintf("%d", time.Since(time.Date(1601, 1, 1, 0, 0, 0, 0, time.UTC)).Microseconds())

	max := 0
	findMax(bar, &max)

	bar.Children = append(bar.Children, &BookmarkNode{
		DateAdded: ts,
		ID:        strconv.Itoa(max + 1),
		Name:      name,
		Type:      "url",
		URL:       url,
	})
}

type BookmarkEntry struct {
	Name string
	URL  string
}

func AddFolder(bf *BookmarkFile, folderName string, entries []BookmarkEntry) {
	bar := bf.Roots["bookmark_bar"]
	ts := fmt.Sprintf("%d", time.Since(time.Date(1601, 1, 1, 0, 0, 0, 0, time.UTC)).Microseconds())

	max := 0
	findMax(bar, &max)

	children := make([]*BookmarkNode, 0, len(entries))
	for i, e := range entries {
		children = append(children, &BookmarkNode{
			DateAdded: ts,
			ID:        strconv.Itoa(max + 2 + i),
			Name:      e.Name,
			Type:      "url",
			URL:       e.URL,
		})
	}

	bar.Children = append(bar.Children, &BookmarkNode{
		Children:     children,
		DateAdded:    ts,
		DateModified: ts,
		ID:           strconv.Itoa(max + 1),
		Name:         folderName,
		Type:         "folder",
	})
}

func Clear(bf *BookmarkFile) {
	bf.Roots["bookmark_bar"].Children = []*BookmarkNode{}
}

func findMax(n *BookmarkNode, max *int) {
	if id, _ := strconv.Atoi(n.ID); id > *max {
		*max = id
	}
	for _, c := range n.Children {
		findMax(c, max)
	}
}
