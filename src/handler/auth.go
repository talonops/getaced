package handler

import (
	"time"

	"getaced.io/src/auth"
	"getaced.io/src/config"
	"getaced.io/src/database"
	"getaced.io/src/middleware"
	"getaced.io/src/structs"

	"github.com/gofiber/fiber/v3"
)

func GoogleAuth(c fiber.Ctx) error {
	url := auth.ConfigGoogle().AuthCodeURL("state")
	return c.Redirect().To(url)
}

func GoogleCallback(c fiber.Ctx) error {
	code := c.Query("code")
	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing code parameter"})
	}

	token, err := auth.ConfigGoogle().Exchange(c.RequestCtx(), code)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "failed to exchange token"})
	}

	userInfo, err := auth.GetUserInfo(token.AccessToken)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get user info"})
	}

	// Upsert user
	var user structs.User
	result := database.DB.Where("google_id = ?", userInfo.ID).First(&user)
	if result.Error != nil {
		now := time.Now()
		trialEnds := now.Add(7 * 24 * time.Hour)
		usageReset := now.Add(30 * 24 * time.Hour)

		user = structs.User{
			GoogleID:           userInfo.ID,
			Email:              userInfo.Email,
			BookmarkFolderName: "school work",
			TrialEndsAt:        &trialEnds,
			UsageResetAt:       &usageReset,
		}
		if err := database.DB.Create(&user).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create user"})
		}
	} else {
		database.DB.Model(&user).Update("email", userInfo.Email)
	}

	// Generate JWT
	jwtToken, err := middleware.GenerateJWT(user.ID, user.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate token"})
	}

	frontendURL := config.Config("FRONTEND_URL")
	if frontendURL == "" {
		return c.JSON(fiber.Map{"token": jwtToken})
	}

	return c.Redirect().To(frontendURL + "/auth/callback?token=" + jwtToken)
}
