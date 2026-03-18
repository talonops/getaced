package middleware

import (
	"time"

	"getaced.io/src/config"
	"getaced.io/src/database"
	"getaced.io/src/structs"

	"github.com/gofiber/fiber/v3"
)

func PaymentRequired(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	now := time.Now()

	// Auto-reset usage counter if reset period has passed
	if user.UsageResetAt != nil && now.After(*user.UsageResetAt) {
		resetAt := now.Add(30 * 24 * time.Hour)
		database.DB.Model(&user).Updates(map[string]interface{}{
			"usage_count":    0,
			"usage_reset_at": &resetAt,
		})
		user.UsageCount = 0
	}

	// Check active subscription or trial (Creem handles trials via "trialing" status)
	isSubActive := user.SubscriptionStatus == "active" || user.SubscriptionStatus == "trialing"

	if !isSubActive {
		return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
			"error":   "subscription required",
			"message": "Please subscribe or start a free trial to use this feature",
		})
	}

	// Check usage limit
	if user.UsageCount >= config.UsageLimit {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"error":          "usage limit reached",
			"usage_count":    user.UsageCount,
			"usage_limit":    config.UsageLimit,
			"usage_reset_at": user.UsageResetAt,
		})
	}

	return c.Next()
}
