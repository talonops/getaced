package handler

import (
	"time"

	"getaced.io/src/database"
	"getaced.io/src/structs"

	"github.com/gofiber/fiber/v3"
)

const UsageLimit = 30

func GetUsage(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	isTrialing := user.SubscriptionStatus == "trialing"
	isSubActive := user.SubscriptionStatus == "active"

	// Daily usage for last 30 days
	since := time.Now().AddDate(0, 0, -30)
	var dailyCounts []struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
	}
	database.DB.Model(&structs.ProcessedFile{}).
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("user_id = ? AND created_at >= ?", userID, since).
		Group("DATE(created_at)").
		Order("date asc").
		Scan(&dailyCounts)

	// Fill in missing days with 0
	days := make([]fiber.Map, 0, 30)
	dayMap := make(map[string]int)
	for _, d := range dailyCounts {
		dayMap[d.Date] = d.Count
	}
	for i := 29; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		count := dayMap[date]
		days = append(days, fiber.Map{"date": date, "count": count})
	}

	return c.JSON(fiber.Map{
		"usage_count":            user.UsageCount,
		"usage_limit":            UsageLimit,
		"usage_reset_at":         user.UsageResetAt,
		"subscription_status":    user.SubscriptionStatus,
		"is_trialing":            isTrialing,
		"is_subscription_active": isSubActive,
		"is_active":              isTrialing || isSubActive,
		"daily_usage":            days,
	})
}
