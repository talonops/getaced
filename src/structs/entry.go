package structs

import (
	"time"

	"gorm.io/gorm"
)

// OnboardingStep is the user's onboarding progression for the app.
type OnboardingStep string

const (
	OnboardingStepChromeProfile OnboardingStep = "chrome_profile"
	OnboardingStepComplete      OnboardingStep = "complete"
)

// User is the main user model stored in SQLite
type User struct {
	gorm.Model
	GoogleID           string         `gorm:"uniqueIndex" json:"google_id"`
	Email              string         `gorm:"uniqueIndex" json:"email"`
	OnboardingStep     OnboardingStep `json:"onboarding_step"`
	CreemCustomerID    string         `json:"creem_customer_id,omitempty"`
	SubscriptionID     string         `json:"subscription_id,omitempty"`
	SubscriptionStatus string         `json:"subscription_status,omitempty"`
	SubscriptionPaidAt *time.Time     `json:"subscription_paid_at,omitempty"`
}

// GoogleResponse is the response from Google's userinfo API
type GoogleResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Verified bool   `json:"verified_email"`
	Picture  string `json:"picture"`
}

// CheckoutRequest is sent to Creem to create a checkout session
type CheckoutRequest struct {
	ProductID  string `json:"product_id"`
	SuccessURL string `json:"success_url"`
	RequestID  string `json:"request_id,omitempty"`
}

// CheckoutResponse is returned by Creem after creating a checkout
type CheckoutResponse struct {
	ID          string `json:"id"`
	CheckoutURL string `json:"checkout_url"`
}

// WebhookEvent is a Creem webhook payload
type WebhookEvent struct {
	EventType string                 `json:"event_type"`
	Object    map[string]interface{} `json:"object"`
}
