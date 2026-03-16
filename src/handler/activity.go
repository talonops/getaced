package handler

import (
	"encoding/json"

	"getaced.io/src/database"
	"getaced.io/src/structs"

	"github.com/gofiber/fiber/v3"
)

func GetActivity(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var files []structs.ProcessedFile
	database.DB.Where("user_id = ?", userID).Order("created_at desc").Limit(20).Find(&files)

	activity := make([]fiber.Map, 0, len(files))
	for _, f := range files {
		var answers []structs.AnswerResult
		if f.Answers != "" {
			json.Unmarshal([]byte(f.Answers), &answers)
		}

		activity = append(activity, fiber.Map{
			"file_name":    f.FileName,
			"answer_count": f.AnswerCount,
			"answers":      answers,
			"processed_at": f.CreatedAt,
		})
	}

	return c.JSON(fiber.Map{"activity": activity})
}
