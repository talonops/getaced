package handler

import (
	"getaced.io/src/config"
	"getaced.io/src/database"
	"getaced.io/src/structs"

	"github.com/gofiber/fiber/v3"
)

func GetMe(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	isTrialing := user.SubscriptionStatus == "trialing"
	isSubActive := user.SubscriptionStatus == "active"

	return c.JSON(fiber.Map{
		"id":                     user.ID,
		"email":                  user.Email,
		"name":                   user.Name,
		"avatar_url":             user.AvatarURL,
		"onboarding_step":        user.OnboardingStep,
		"subscription_status":    user.SubscriptionStatus,
		"subscription_id":        user.SubscriptionID,
		"has_drive_connected":    user.DriveRefreshToken != "",
		"watch_folder_id":        user.WatchFolderID,
		"bookmark_folder_name":   user.BookmarkFolderName,
		"usage_count":            user.UsageCount,
		"usage_limit":            config.UsageLimit,
		"is_trialing":            isTrialing,
		"is_subscription_active": isSubActive,
		"is_active":              isTrialing || isSubActive,
	})
}
