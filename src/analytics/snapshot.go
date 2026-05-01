package analytics

import (
	"log"
	"time"

	"getaced.io/src/database"
	"getaced.io/src/structs"
)

// GenerateDailySnapshot creates or updates today's aggregate row.
// Safe to call repeatedly — upserts by date.
func GenerateDailySnapshot() {
	today := time.Now().UTC().Format("2006-01-02")
	todayStart := today + " 00:00:00"
	todayEnd := today + " 23:59:59"
	weekAgo := time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02") + " 00:00:00"

	snap := structs.AnalyticsDailySnapshot{Date: today}

	// Total users
	var totalUsers int64
	database.DB.Model(&structs.User{}).Count(&totalUsers)
	snap.TotalUsers = int(totalUsers)

	// DAU: distinct users with user_active event today
	var dau int64
	database.DB.Model(&structs.AnalyticsEvent{}).
		Where("event = ? AND created_at BETWEEN ? AND ?", "user_active", todayStart, todayEnd).
		Distinct("user_id").Count(&dau)
	snap.DAU = int(dau)

	// WAU: distinct users with user_active event in last 7 days
	var wau int64
	database.DB.Model(&structs.AnalyticsEvent{}).
		Where("event = ? AND created_at >= ?", "user_active", weekAgo).
		Distinct("user_id").Count(&wau)
	snap.WAU = int(wau)

	// Files processed today
	var files int64
	database.DB.Model(&structs.AnalyticsEvent{}).
		Where("event = ? AND created_at BETWEEN ? AND ?", "file_processed", todayStart, todayEnd).
		Count(&files)
	snap.FilesProcessed = int(files)

	// Token usage today
	var tokensIn, tokensOut *int64
	database.DB.Model(&structs.AnalyticsEvent{}).
		Select("SUM(tokens_input)").
		Where("event = ? AND created_at BETWEEN ? AND ?", "file_processed", todayStart, todayEnd).
		Row().Scan(&tokensIn)
	database.DB.Model(&structs.AnalyticsEvent{}).
		Select("SUM(tokens_output)").
		Where("event = ? AND created_at BETWEEN ? AND ?", "file_processed", todayStart, todayEnd).
		Row().Scan(&tokensOut)
	if tokensIn != nil {
		snap.TokensInput = int(*tokensIn)
	}
	if tokensOut != nil {
		snap.TokensOutput = int(*tokensOut)
	}

	// Upsert: create or update today's row
	var existing structs.AnalyticsDailySnapshot
	if err := database.DB.Where("date = ?", today).First(&existing).Error; err != nil {
		if err := database.DB.Create(&snap).Error; err != nil {
			log.Printf("analytics: failed to create daily snapshot: %v", err)
		}
	} else {
		if err := database.DB.Model(&existing).Updates(map[string]interface{}{
			"total_users":     snap.TotalUsers,
			"dau":             snap.DAU,
			"wau":             snap.WAU,
			"files_processed": snap.FilesProcessed,
			"tokens_input":    snap.TokensInput,
			"tokens_output":   snap.TokensOutput,
		}).Error; err != nil {
			log.Printf("analytics: failed to update daily snapshot: %v", err)
		}
	}

	log.Printf("analytics: snapshot for %s — users=%d dau=%d wau=%d files=%d",
		today, snap.TotalUsers, snap.DAU, snap.WAU, snap.FilesProcessed)
}
