package handler

import (
	"time"

	"getaced.io/src/config"
	"getaced.io/src/creem"
	"getaced.io/src/database"
	"getaced.io/src/structs"

	"github.com/gofiber/fiber/v3"
)

var CreemClient *creem.Client

func CreateCheckout(c fiber.Ctx) error {
	var body struct {
		ProductID  string `json:"product_id"`
		SuccessURL string `json:"success_url"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	checkout, err := CreemClient.CreateCheckout(structs.CheckoutRequest{
		ProductID:  body.ProductID,
		SuccessURL: body.SuccessURL,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create checkout"})
	}

	return c.JSON(fiber.Map{"checkout_url": checkout.CheckoutURL})
}

func CreemWebhook(c fiber.Ctx) error {
	signature := c.Get("creem-signature")
	body := c.Body()

	if !creem.VerifySignature(body, signature, config.Config("CREEM_WEBHOOK_SECRET")) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid signature"})
	}

	var event structs.WebhookEvent
	if err := c.Bind().JSON(&event); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid event payload"})
	}

	obj := event.Object
	customerID, _ := obj["customer_id"].(string)
	subscriptionID, _ := obj["id"].(string)

	if customerID == "" {
		return c.SendStatus(fiber.StatusOK)
	}

	var user structs.User
	if err := database.DB.Where("creem_customer_id = ?", customerID).First(&user).Error; err != nil {
		return c.SendStatus(fiber.StatusOK)
	}

	switch event.EventType {
	case "checkout.completed":
		database.DB.Model(&user).Updates(map[string]interface{}{
			"creem_customer_id": customerID,
			"subscription_id":   subscriptionID,
		})
	case "subscription.active", "subscription.paid":
		now := time.Now()
		database.DB.Model(&user).Updates(map[string]interface{}{
			"subscription_status":  "active",
			"subscription_paid_at": &now,
		})
	case "subscription.canceled":
		database.DB.Model(&user).Update("subscription_status", "canceled")
	case "subscription.past_due":
		database.DB.Model(&user).Update("subscription_status", "past_due")
	case "subscription.expired":
		database.DB.Model(&user).Update("subscription_status", "expired")
	}

	return c.SendStatus(fiber.StatusOK)
}
