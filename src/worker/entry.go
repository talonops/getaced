package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"getaced.io/src/analytics"
	"getaced.io/src/bookmarks"
	"getaced.io/src/config"
	"getaced.io/src/containers"
	"getaced.io/src/crypto"
	"getaced.io/src/database"
	"getaced.io/src/drive"
	openaiPkg "getaced.io/src/openai"
	"getaced.io/src/structs"

	"github.com/google/uuid"
	driveapi "google.golang.org/api/drive/v3"
	"gorm.io/gorm"
)

var (
	openaiClient *openaiPkg.Client
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	cleanupHooks []func()
)

// RegisterCleanupHook adds a function to be called during session cleanup ticks
func RegisterCleanupHook(fn func()) {
	cleanupHooks = append(cleanupHooks, fn)
}

const (
	maxConcurrent  = 10
	maxFileRetries = 3
	debounceDelay  = 2 * time.Second
)

// Debounce queue: webhook notifications enqueue user IDs here
var (
	pendingUsers = make(map[uint]time.Time)
	pendingMu    sync.Mutex
)

// EnqueueUser is called by the Drive webhook handler to schedule processing
func EnqueueUser(userID uint) {
	pendingMu.Lock()
	defer pendingMu.Unlock()
	if _, exists := pendingUsers[userID]; !exists {
		pendingUsers[userID] = time.Now()
	}
}

func Start(client *openaiPkg.Client) {
	openaiClient = client

	var ctx context.Context
	ctx, cancel = context.WithCancel(context.Background())

	wg.Add(1)
	go func() {
		defer wg.Done()
		pollLoop(ctx)
	}()

	log.Println("worker started (push notifications only)")
}

func Stop() {
	if cancel != nil {
		cancel()
	}
	wg.Wait()
	log.Println("worker stopped")
}

func pollLoop(ctx context.Context) {
	// 1. Debounce ticker — process webhook-enqueued users after delay
	debounceTicker := time.NewTicker(3 * time.Second)
	defer debounceTicker.Stop()

	// 2. Watch renewal — re-register expiring watches
	renewTicker := time.NewTicker(6 * time.Hour)
	defer renewTicker.Stop()

	// 3. Session cleanup
	sessionTicker := time.NewTicker(30 * time.Second)
	defer sessionTicker.Stop()

	// 4. Analytics daily snapshot — hourly, also run on startup
	snapshotTicker := time.NewTicker(1 * time.Hour)
	defer snapshotTicker.Stop()
	analytics.GenerateDailySnapshot()

	for {
		select {
		case <-ctx.Done():
			return
		case <-debounceTicker.C:
			processPendingUsers(ctx)
		case <-renewTicker.C:
			renewExpiringWatches()
		case <-sessionTicker.C:
			containers.CleanupExpiredSessions()
			for _, fn := range cleanupHooks {
				fn()
			}
		case <-snapshotTicker.C:
			analytics.GenerateDailySnapshot()
		}
	}
}

// processPendingUsers pops users from the debounce queue and processes them
func processPendingUsers(ctx context.Context) {
	pendingMu.Lock()
	ready := make(map[uint]time.Time)
	now := time.Now()
	for userID, enqueuedAt := range pendingUsers {
		if now.Sub(enqueuedAt) >= debounceDelay {
			ready[userID] = enqueuedAt
			delete(pendingUsers, userID)
		}
	}
	pendingMu.Unlock()

	if len(ready) == 0 {
		return
	}

	sem := make(chan struct{}, maxConcurrent)
	var pwg sync.WaitGroup

	for userID := range ready {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var user structs.User
		if err := database.DB.First(&user, userID).Error; err != nil {
			continue
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

	// Get files either via Changes API (push) or ListNewFiles (fallback)
	var files []drive.DriveFile
	if user.DrivePageToken != "" {
		files, err = getFilesViaChanges(srv, &user)
	} else {
		files, err = getFilesViaPolling(srv, &user)
	}
	if err != nil {
		return err
	}

	containerName := fmt.Sprintf("getaced-%d", user.ID)
	folderName := user.BookmarkFolderName
	if folderName == "" {
		folderName = "school work"
	}

	// Reset usage if the reset period has passed
	if user.UsageResetAt != nil && time.Now().After(*user.UsageResetAt) {
		newReset := time.Now().Add(30 * 24 * time.Hour)
		database.DB.Model(&user).Updates(map[string]interface{}{
			"usage_count":    0,
			"usage_reset_at": &newReset,
		})
		user.UsageCount = 0
		user.UsageResetAt = &newReset
	}

	for _, file := range files {
		if user.UsageCount >= config.UsageLimit {
			log.Printf("worker: user %d reached usage limit", user.ID)
			break
		}

		// Skip already processed files
		var count int64
		database.DB.Model(&structs.ProcessedFile{}).Where("user_id = ? AND file_id = ?", user.ID, file.ID).Count(&count)
		if count > 0 {
			continue
		}

		// Skip files that have failed too many times
		var failed structs.FailedFile
		if err := database.DB.Where("user_id = ? AND file_id = ?", user.ID, file.ID).First(&failed).Error; err == nil {
			if failed.Failures >= maxFileRetries {
				log.Printf("worker: skipping file %s for user %d (failed %d times)", file.Name, user.ID, failed.Failures)
				continue
			}
		}

		data, mimeType, err := drive.DownloadFile(srv, file.ID)
		if err != nil {
			log.Printf("worker: failed to download file %s for user %d: %v", file.Name, user.ID, err)
			recordFailure(user.ID, file.ID, file.Name)
			continue
		}

		var answers *structs.AnswerResponse
		var tokenUsage *openaiPkg.TokenUsage

		switch {
		case mimeType == "text/html":
			text := openaiPkg.StripHTML(data)
			answers, tokenUsage, err = openaiClient.ProcessText(text, user.CustomPrompt)
		case mimeType == "multipart/related", mimeType == "message/rfc822", strings.HasSuffix(strings.ToLower(file.Name), ".mhtml"), strings.HasSuffix(strings.ToLower(file.Name), ".mht"):
			htmlBody, extractErr := openaiPkg.ExtractMHTML(data)
			if extractErr != nil {
				log.Printf("worker: failed to extract MHTML for file %s (user %d): %v", file.Name, user.ID, extractErr)
				recordFailure(user.ID, file.ID, file.Name)
				continue
			}
			text := openaiPkg.StripHTML(htmlBody)
			answers, tokenUsage, err = openaiClient.ProcessText(text, user.CustomPrompt)
		case mimeType == "image/png":
			answers, tokenUsage, err = openaiClient.ProcessImage(data, user.CustomPrompt)
		default:
			log.Printf("worker: skipping unsupported file type %s for file %s", mimeType, file.Name)
			continue
		}

		if err != nil {
			log.Printf("worker: GPT processing failed for file %s (user %d): %v", file.Name, user.ID, err)
			recordFailure(user.ID, file.ID, file.Name)
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
		answersJSON, _ := json.Marshal(answers.Answers)
		database.DB.Create(&structs.ProcessedFile{
			UserID:      user.ID,
			FileID:      file.ID,
			FileName:    file.Name,
			AnswerCount: len(answers.Answers),
			Answers:     string(answersJSON),
		})

		// Clear any previous failure record
		database.DB.Where("user_id = ? AND file_id = ?", user.ID, file.ID).Delete(&structs.FailedFile{})

		database.DB.Model(&user).Update("usage_count", gorm.Expr("usage_count + 1"))
		user.UsageCount++

		if tokenUsage != nil {
			analytics.Track("file_processed", user.ID, analytics.WithTokens(tokenUsage.PromptTokens, tokenUsage.CompletionTokens))
		} else {
			analytics.Track("file_processed", user.ID)
		}

		log.Printf("worker: processed file %s for user %d (%d answers)", file.Name, user.ID, len(answers.Answers))
	}

	return nil
}

// getFilesViaChanges uses the Changes API (for users with push notifications set up)
func getFilesViaChanges(srv *driveapi.Service, user *structs.User) ([]drive.DriveFile, error) {
	allFiles, newToken, err := drive.ListChanges(srv, user.DrivePageToken)
	if err != nil {
		return nil, fmt.Errorf("list changes: %w", err)
	}

	// Update the page token for next time
	if newToken != "" {
		database.DB.Model(user).Update("drive_page_token", newToken)
		user.DrivePageToken = newToken
	}

	// Filter to only files in the user's watch folder
	var filtered []drive.DriveFile
	for _, f := range allFiles {
		for _, parent := range f.Parents {
			if parent == user.WatchFolderID {
				filtered = append(filtered, f)
				break
			}
		}
	}

	return filtered, nil
}

// getFilesViaPolling uses ListNewFiles (fallback for users without push notifications)
func getFilesViaPolling(srv *driveapi.Service, user *structs.User) ([]drive.DriveFile, error) {
	since := time.Now().Add(-24 * time.Hour)
	var lastFile structs.ProcessedFile
	if err := database.DB.Where("user_id = ?", user.ID).Order("created_at desc").First(&lastFile).Error; err == nil {
		since = lastFile.CreatedAt
	}

	files, err := drive.ListNewFiles(srv, user.WatchFolderID, since)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}

	return files, nil
}

// recordFailure tracks a file processing failure for retry limiting
func recordFailure(userID uint, fileID, fileName string) {
	var failed structs.FailedFile
	if err := database.DB.Where("user_id = ? AND file_id = ?", userID, fileID).First(&failed).Error; err != nil {
		// New failure record
		database.DB.Create(&structs.FailedFile{
			UserID:   userID,
			FileID:   fileID,
			FileName: fileName,
			Failures: 1,
		})
	} else {
		// Increment existing
		database.DB.Model(&failed).Update("failures", failed.Failures+1)
	}
	analytics.Track("file_failed", userID)
}

// RegisterUserWatch sets up Drive push notifications for a user
func RegisterUserWatch(user structs.User, srv *driveapi.Service) error {
	pageToken, err := drive.GetStartPageToken(srv)
	if err != nil {
		return fmt.Errorf("get start page token: %w", err)
	}

	channelID := uuid.New().String()
	webhookURL := config.Config("WEBHOOK_BASE_URL") + "/v1/webhooks/drive"

	channelToken := config.Config("DRIVE_WEBHOOK_TOKEN")
	expiry, err := drive.RegisterWatch(srv, channelID, webhookURL, pageToken, channelToken)
	if err != nil {
		return fmt.Errorf("register watch: %w", err)
	}

	expiryTime := time.UnixMilli(expiry)
	database.DB.Model(&user).Updates(map[string]interface{}{
		"drive_page_token":     pageToken,
		"drive_channel_id":     channelID,
		"drive_channel_expiry": &expiryTime,
	})

	log.Printf("worker: registered drive watch for user %d (channel %s, expires %s)", user.ID, channelID, expiryTime.Format(time.RFC3339))
	return nil
}

// StopUserWatch stops Drive push notifications for a user
func StopUserWatch(user structs.User) {
	if user.DriveChannelID == "" {
		return
	}

	refreshToken, err := crypto.Decrypt(user.DriveRefreshToken, config.Config("DRIVE_TOKEN_ENCRYPT_KEY"))
	if err != nil {
		log.Printf("worker: failed to decrypt token for user %d to stop watch: %v", user.ID, err)
		return
	}

	srv, err := drive.NewServiceFromRefreshToken(refreshToken)
	if err != nil {
		log.Printf("worker: failed to create drive service for user %d to stop watch: %v", user.ID, err)
		return
	}

	if err := drive.StopWatch(srv, user.DriveChannelID, ""); err != nil {
		log.Printf("worker: failed to stop watch for user %d: %v", user.ID, err)
	}

	database.DB.Model(&user).Updates(map[string]interface{}{
		"drive_channel_id":     "",
		"drive_channel_expiry": nil,
	})

	log.Printf("worker: stopped drive watch for user %d", user.ID)
}

// renewExpiringWatches re-registers watches that expire within the next 24 hours
func renewExpiringWatches() {
	cutoff := time.Now().Add(24 * time.Hour)

	var users []structs.User
	database.DB.Where(
		"drive_channel_id != '' AND drive_channel_expiry IS NOT NULL AND drive_channel_expiry < ? AND "+
			"drive_refresh_token != '' AND watch_folder_id != '' AND "+
			"subscription_status IN ('active','trialing')",
		cutoff,
	).Find(&users)

	for _, user := range users {
		refreshToken, err := crypto.Decrypt(user.DriveRefreshToken, config.Config("DRIVE_TOKEN_ENCRYPT_KEY"))
		if err != nil {
			log.Printf("worker: failed to decrypt token for watch renewal (user %d): %v", user.ID, err)
			continue
		}

		srv, err := drive.NewServiceFromRefreshToken(refreshToken)
		if err != nil {
			log.Printf("worker: failed to create drive service for watch renewal (user %d): %v", user.ID, err)
			continue
		}

		// Stop old watch (best effort)
		drive.StopWatch(srv, user.DriveChannelID, "")

		// Register new watch
		if err := RegisterUserWatch(user, srv); err != nil {
			log.Printf("worker: failed to renew watch for user %d: %v", user.ID, err)
		}
	}
}
