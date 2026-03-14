package handler

import (
	"getaced.io/src/auth"
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
		user = structs.User{
			GoogleID: userInfo.ID,
			Email:    userInfo.Email,
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

	return c.JSON(fiber.Map{"token": jwtToken})
}
