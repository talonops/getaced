package bookmarks

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"getaced.io/src/containers"
	"getaced.io/src/structs"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"

	"github.com/google/uuid"
)

type BookmarkEntry struct {
	Name string
}

// chromeTimestamp returns microseconds since January 1, 1601 (Chrome's epoch)
func chromeTimestamp() string {
	epoch := time.Date(1601, 1, 1, 0, 0, 0, 0, time.UTC)
	return fmt.Sprintf("%d", time.Since(epoch).Microseconds())
}

func AnswersToBookmarks(answers *structs.AnswerResponse) []BookmarkEntry {
	entries := make([]BookmarkEntry, 0, len(answers.Answers))
	for _, a := range answers.Answers {
		entries = append(entries, BookmarkEntry{
			Name: fmt.Sprintf("%d. %s", a.Number, a.Answer),
		})
	}
	return entries
}

func execWait(containerName string, cmd []string) error {
	op, err := containers.Client.ExecInstance(containerName, api.InstanceExecPost{
		Command:     cmd,
		WaitForWS:   true,
		Interactive: false,
	}, nil)
	if err != nil {
		return fmt.Errorf("exec %v: %w", cmd, err)
	}
	return op.Wait()
}

func stopContainer(name string) error {
	inst, _, err := containers.Client.GetInstance(name)
	if err != nil {
		return fmt.Errorf("get instance state: %w", err)
	}
	if inst.StatusCode != api.Running {
		return nil // already stopped
	}
	op, err := containers.Client.UpdateInstanceState(name, api.InstanceStatePut{Action: "stop", Force: true}, "")
	if err != nil {
		return fmt.Errorf("stop container: %w", err)
	}
	return op.Wait()
}

func startContainer(name string) error {
	op, err := containers.Client.UpdateInstanceState(name, api.InstanceStatePut{Action: "start"}, "")
	if err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	return op.Wait()
}

func UpdateBookmarks(containerName, folderName string, entries []BookmarkEntry) error {
	const bookmarksPath = "/root/.config/google-chrome/Default/Bookmarks"

	// 1. Read bookmarks while container is running
	reader, _, err := containers.Client.GetInstanceFile(containerName, bookmarksPath)
	if err != nil {
		return fmt.Errorf("read bookmarks file: %w", err)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read bookmarks data: %w", err)
	}

	var bookmarks map[string]interface{}
	if err := json.Unmarshal(data, &bookmarks); err != nil {
		return fmt.Errorf("parse bookmarks: %w", err)
	}

	roots, ok := bookmarks["roots"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid bookmarks structure: missing roots")
	}

	bar, ok := roots["bookmark_bar"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid bookmarks structure: missing bookmark_bar")
	}

	children, _ := bar["children"].([]interface{})

	// Find or create the target folder
	var folder map[string]interface{}
	for _, child := range children {
		c, ok := child.(map[string]interface{})
		if !ok {
			continue
		}
		if c["type"] == "folder" && c["name"] == folderName {
			folder = c
			break
		}
	}

	if folder == nil {
		folder = map[string]interface{}{
			"children":   []interface{}{},
			"date_added": chromeTimestamp(),
			"guid":       uuid.New().String(),
			"id":         fmt.Sprintf("%d", len(children)+1),
			"name":       folderName,
			"type":       "folder",
		}
		children = append(children, folder)
		bar["children"] = children
	}

	// Build new bookmark entries as folders (not URLs)
	newChildren := make([]interface{}, 0, len(entries))
	for i, entry := range entries {
		ts := chromeTimestamp()
		newChildren = append(newChildren, map[string]interface{}{
			"children":   []interface{}{},
			"date_added": ts,
			"guid":       uuid.New().String(),
			"id":         fmt.Sprintf("%d", i+100),
			"name":       entry.Name,
			"type":       "folder",
		})
	}
	folder["children"] = newChildren

	output, err := json.MarshalIndent(bookmarks, "", "   ")
	if err != nil {
		return fmt.Errorf("marshal bookmarks: %w", err)
	}

	// 2. Stop the container (kills Chrome so it releases bookmarks file)
	if err := stopContainer(containerName); err != nil {
		return fmt.Errorf("stop container: %w", err)
	}

	// 3. Write bookmarks while container is stopped
	err = containers.Client.CreateInstanceFile(containerName, bookmarksPath, lxd.InstanceFileArgs{
		Content: strings.NewReader(string(output)),
		Type:    "file",
	})
	if err != nil {
		// Try to restart even if write fails
		if startErr := startContainer(containerName); startErr != nil {
			log.Printf("bookmarks: failed to restart container %s after write failure: %v", containerName, startErr)
		}
		return fmt.Errorf("write bookmarks file: %w", err)
	}

	return nil
}

func SyncChrome(containerName string) error {
	// Start container back up
	if err := startContainer(containerName); err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	// Inject latest start.sh
	err := containers.Client.CreateInstanceFile(containerName, "/root/start.sh", lxd.InstanceFileArgs{
		Content: strings.NewReader(string(containers.StartScript)),
		Type:    "file",
		Mode:    0755,
	})
	if err != nil {
		return fmt.Errorf("inject start.sh: %w", err)
	}
	// Run sync
	if err := execWait(containerName, []string{"/root/start.sh", "sync"}); err != nil {
		return fmt.Errorf("sync chrome: %w", err)
	}
	// Stop container after sync is done
	if err := stopContainer(containerName); err != nil {
		return fmt.Errorf("stop container after sync: %w", err)
	}
	return nil
}
