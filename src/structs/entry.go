package structs

import (
	"errors"
	"sync"
	"time"

	"gorm.io/gorm"
)

// OnboardingStep is the user's onboarding progression for the app.
type OnboardingStep string

const (
	OnboardingStepChrome   OnboardingStep = "OnboardingStepChrome"
	OnboardingStepComplete OnboardingStep = "complete"
)

// User is the main user model stored in SQLite
type User struct {
	gorm.Model
	GoogleID           string         `gorm:"uniqueIndex" json:"google_id"`
	Email              string         `gorm:"uniqueIndex" json:"email"`
	OnboardingStep     OnboardingStep `json:"onboarding_step" gorm:"default:OnboardingStepChrome"`
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

// PortPool is for containers port management
type PortPool struct {
	MU        sync.Mutex
	Available []int          // [6081, 6082, ... 6100]
	Used      map[int]string // port -> userID
}

func NewPortPool(start, count int) *PortPool {
	ports := make([]int, count)
	for i := range ports {
		ports[i] = start + i // 6081, 6082, ..., 6100
	}
	return &PortPool{
		Available: ports,
		Used:      make(map[int]string),
	}
}

func (p *PortPool) AcquirePort(userID string) (int, error) {
	p.MU.Lock()
	defer p.MU.Unlock()
	if len(p.Available) == 0 {
		return 0, errors.New("no ports available")
	}
	port := p.Available[0]
	p.Available = p.Available[1:]
	p.Used[port] = userID
	return port, nil
}

func (p *PortPool) ReleasePort(port int) {
	p.MU.Lock()
	defer p.MU.Unlock()
	delete(p.Used, port)
	p.Available = append(p.Available, port)
}
