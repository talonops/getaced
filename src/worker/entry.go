package worker

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"getaced.io/src/bookmarks"
	"getaced.io/src/config"
	"getaced.io/src/containers"
	"getaced.io/src/crypto"
	"getaced.io/src/database"
	"getaced.io/src/drive"
	openaiPkg "getaced.io/src/openai"
	"getaced.io/src/structs"
)

var (
	openaiClient *openaiPkg.Client
	cancel       context.CancelFunc
	wg           sync.WaitGroup
)

const maxConcurrent = 10

func Start(client *openaiPkg.Client) {
	openaiClient = client

	var ctx context.Context
	ctx, cancel = context.WithCancel(context.Background())

	pollInterval := 60 * time.Second
	if v := config.Config("POLL_INTERVAL"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			pollInterval = time.Duration(secs) * time.Second
		}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		pollLoop(ctx, pollInterval)
	}()

	log.Printf("worker started (poll interval: %s)", pollInterval)
}

func Stop() {
	if cancel != nil {
		cancel()
	}
	wg.Wait()
	log.Println("worker stopped")
}

func pollLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Also run session cleanup every 30s
	sessionTicker := time.NewTicker(30 * time.Second)
	defer sessionTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sessionTicker.C:
			containers.CleanupExpiredSessions()
		case <-ticker.C:
			processAllUsers(ctx)
		}
	}
}

func processAllUsers(ctx context.Context) {
	now := time.Now()

	var users []structs.User
	database.DB.Where(
		"drive_refresh_token != '' AND watch_folder_id != '' AND usage_count < ? AND "+
			"(subscription_status = 'active' OR (trial_ends_at IS NOT NULL AND trial_ends_at > ?))",
		30, now,
	).Find(&users)

	if len(users) == 0 {
		return
	}

	sem := make(chan struct{}, maxConcurrent)
	var pwg sync.WaitGroup

	for _, user := range users {
		select {
		case <-ctx.Done():
			return
		default:
		}

		sem <- struct{}{}
		pwg.Add(1)
		go func(u structs.User) {
			defer pwg.Done()
			defer func() { <-sem }()

			if err := processUser(u); err != nil {
				log.Printf("worker: error processing user %d: %v", u.ID, err)
			}
		}(user)
	}

	pwg.Wait()
}

func processUser(user structs.User) error {
	refreshToken, err := crypto.Decrypt(user.DriveRefreshToken, config.Config("DRIVE_TOKEN_ENCRYPT_KEY"))
	if err != nil {
		return fmt.Errorf("decrypt token: %w", err)
	}

	srv, err := drive.NewServiceFromRefreshToken(refreshToken)
	if err != nil {
		return fmt.Errorf("create drive service: %w", err)
	}

	// Get the most recent processed file time, or default to 24h ago
	since := time.Now().Add(-24 * time.Hour)
	var lastFile structs.ProcessedFile
	if err := database.DB.Where("user_id = ?", user.ID).Order("created_at desc").First(&lastFile).Error; err == nil {
		since = lastFile.CreatedAt
	}

	files, err := drive.ListNewFiles(srv, user.WatchFolderID, since)
	if err != nil {
		return fmt.Errorf("list files: %w", err)
	}

	containerName := fmt.Sprintf("getaced-%d", user.ID)
	folderName := user.BookmarkFolderName
	if folderName == "" {
		folderName = "school work"
	}

	for _, file := range files {
		if user.UsageCount >= 30 {
			log.Printf("worker: user %d reached usage limit", user.ID)
			break
		}

		// Skip already processed files
		var count int64
		database.DB.Model(&structs.ProcessedFile{}).Where("user_id = ? AND file_id = ?", user.ID, file.ID).Count(&count)
		if count > 0 {
			continue
		}

		data, mimeType, err := drive.DownloadFile(srv, file.ID)
		if err != nil {
			log.Printf("worker: failed to download file %s for user %d: %v", file.Name, user.ID, err)
			continue
		}

		var answers *structs.AnswerResponse

		switch {
		case mimeType == "text/html":
			text := openaiPkg.StripHTML(data)
			answers, err = openaiClient.ProcessText(text, user.CustomPrompt)
		case mimeType == "image/png":
			answers, err = openaiClient.ProcessImage(data, user.CustomPrompt)
		default:
			log.Printf("worker: skipping unsupported file type %s for file %s", mimeType, file.Name)
			continue
		}

		if err != nil {
			log.Printf("worker: GPT processing failed for file %s (user %d): %v", file.Name, user.ID, err)
			continue
		}

		if len(answers.Answers) == 0 {
			log.Printf("worker: no answers extracted from file %s (user %d)", file.Name, user.ID)
			continue
		}

		entries := bookmarks.AnswersToBookmarks(answers)

		if err := bookmarks.UpdateBookmarks(containerName, folderName, entries); err != nil {
			log.Printf("worker: failed to update bookmarks for user %d: %v", user.ID, err)
			continue
		}

		if err := bookmarks.SyncChrome(containerName); err != nil {
			log.Printf("worker: failed to sync chrome for user %d: %v", user.ID, err)
		}

		// Record processed file and increment usage
		database.DB.Create(&structs.ProcessedFile{
			UserID:   user.ID,
			FileID:   file.ID,
			FileName: file.Name,
		})

		database.DB.Model(&user).Update("usage_count", user.UsageCount+1)
		user.UsageCount++

		log.Printf("worker: processed file %s for user %d (%d answers)", file.Name, user.ID, len(answers.Answers))
	}

	return nil
}
