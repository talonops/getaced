package handler

import (
	"fmt"
	"log"

	"getaced.io/src/config"
	"getaced.io/src/crypto"
	"getaced.io/src/database"
	"getaced.io/src/drive"
	"getaced.io/src/structs"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/oauth2"
)

func DriveAuth(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	if user.OnboardingStep != structs.OnboardingStepDrive {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid onboarding step"})
	}

	cfg := drive.OAuthConfig()
	url := cfg.AuthCodeURL(
		fmt.Sprintf("drive:%d", userID),
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	return c.JSON(fiber.Map{"url": url})
}

func DriveCallback(c fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing code"})
	}

	var userID uint
	if _, err := fmt.Sscanf(state, "drive:%d", &userID); err != nil || userID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid state"})
	}

	cfg := drive.OAuthConfig()
	token, err := cfg.Exchange(c.Context(), code)
	if err != nil {
		log.Printf("drive oauth exchange error: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "failed to exchange token"})
	}

	if token.RefreshToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no refresh token received"})
	}

	encryptedToken, err := crypto.Encrypt(token.RefreshToken, config.Config("DRIVE_TOKEN_ENCRYPT_KEY"))
	if err != nil {
		log.Printf("encrypt refresh token error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to store token"})
	}

	database.DB.Model(&structs.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"drive_refresh_token": encryptedToken,
		"onboarding_step":     structs.OnboardingStepChrome,
	})

	frontendURL := config.Config("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "https://getaced.io"
	}

	return c.Redirect().To(frontendURL + "/onboarding?step=chrome")
}
