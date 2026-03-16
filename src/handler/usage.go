package handler

import (
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

	return c.JSON(fiber.Map{
		"usage_count":            user.UsageCount,
		"usage_limit":            UsageLimit,
		"usage_reset_at":         user.UsageResetAt,
		"subscription_status":    user.SubscriptionStatus,
		"is_trialing":            isTrialing,
		"is_subscription_active": isSubActive,
		"is_active":              isTrialing || isSubActive,
	})
}
