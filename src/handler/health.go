package handler

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
)

var startTime = time.Now()

func Health(c fiber.Ctx) error {
	uptime := time.Since(startTime)
	hours := int(uptime.Hours())
	days := hours / 24
	hours = hours % 24
	mins := int(uptime.Minutes()) % 60

	return c.JSON(fiber.Map{
		"status": "ok",
		"uptime": fmt.Sprintf("%dd %dh %dm", days, hours, mins),
	})
}
