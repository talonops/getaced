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
	OnboardingStepDrive       OnboardingStep = "OnboardingStepDrive"
	OnboardingStepWatchFolder OnboardingStep = "OnboardingStepWatchFolder"
	OnboardingStepSession     OnboardingStep = "OnboardingStepSession"
	OnboardingComplete        OnboardingStep = "OnboardingComplete"
)

// User is the main user model stored in SQLite
type User struct {
	gorm.Model
	GoogleID           string         `gorm:"uniqueIndex" json:"google_id"`
	Email              string         `gorm:"uniqueIndex" json:"email"`
	Name               string         `json:"name,omitempty"`
	AvatarURL          string         `json:"avatar_url,omitempty"`
	LastActiveAt       *time.Time     `json:"last_active_at,omitempty"`
	OnboardingStep     OnboardingStep `json:"onboarding_step" gorm:"default:OnboardingStepDrive"`
	CreemCustomerID    string         `json:"creem_customer_id,omitempty"`
	SubscriptionID     string         `json:"subscription_id,omitempty"`
	SubscriptionStatus string         `json:"subscription_status,omitempty"`
	SubscriptionPaidAt *time.Time     `json:"subscription_paid_at,omitempty"`

	// Drive integration
	DriveEmail         string `json:"drive_email,omitempty"`
	DriveRefreshToken  string `gorm:"type:text" json:"-"`
	WatchFolderID      string `json:"watch_folder_id,omitempty"`
	BookmarkFolderName string `gorm:"default:school work" json:"bookmark_folder_name,omitempty"`
	CustomPrompt       string `gorm:"type:text" json:"custom_prompt,omitempty"`

	// Drive push notifications
	DrivePageToken     string     `json:"drive_page_token,omitempty"`
	DriveChannelID     string     `json:"drive_channel_id,omitempty"`
	DriveChannelExpiry *time.Time `json:"drive_channel_expiry,omitempty"`

	// Payment enforcement
	UsageCount   int        `json:"usage_count" gorm:"default:0"`
	UsageResetAt *time.Time `json:"usage_reset_at,omitempty"`
	TrialEndsAt  *time.Time `json:"trial_ends_at,omitempty"`
}

// GoogleResponse is the response from Google's userinfo API
type GoogleResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
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

// BillingPortalResponse is returned by Creem for customer portal access
type BillingPortalResponse struct {
	CustomerPortalLink string `json:"customer_portal_link"`
}

// WebhookEvent is a Creem webhook payload
type WebhookEvent struct {
	ID        string                 `json:"id"`
	EventType string                 `json:"eventType"`
	Object    map[string]interface{} `json:"object"`
}

// SetupSession is used to store info while user is setting up a browser session
type SetupSession struct {
	UserID    uint
	Container string
	IPSuffix  int
	CreatedAt time.Time
	ExpiresAt time.Time
}

// ProcessedFile tracks Drive files already processed to avoid reprocessing
type ProcessedFile struct {
	gorm.Model
	UserID      uint   `gorm:"index"`
	FileID      string `gorm:"index"`
	FileName    string
	AnswerCount int    `json:"answer_count" gorm:"default:0"`
	Answers     string `json:"answers" gorm:"type:text"`
}

// AnswerResult is a single extracted answer from GPT
type AnswerResult struct {
	Number int    `json:"number"`
	Answer string `json:"answer"`
}

// AnswerResponse is the full GPT response with all answers
type AnswerResponse struct {
	Answers []AnswerResult `json:"answers"`
}

// ProcessedWebhookEvent tracks Creem webhook events to ensure idempotency
type ProcessedWebhookEvent struct {
	gorm.Model
	EventID string `gorm:"uniqueIndex"`
}

// FailedFile tracks files that failed processing to limit retries
type FailedFile struct {
	gorm.Model
	UserID   uint   `gorm:"index"`
	FileID   string `gorm:"index"`
	FileName string
	Failures int `gorm:"default:0"`
}

// AnalyticsEvent is an append-only log of business events.
type AnalyticsEvent struct {
	ID           uint      `gorm:"primaryKey;autoIncrement"`
	CreatedAt    time.Time `gorm:"index;autoCreateTime"`
	UserID       uint      `gorm:"index"`
	Event        string    `gorm:"index;not null"`
	StepName     string    `gorm:"default:null"`
	PlanType     string    `gorm:"default:null"`
	Amount       int       `gorm:"default:0"`
	TokensInput  int       `gorm:"default:0"`
	TokensOutput int       `gorm:"default:0"`
}

// AnalyticsDailySnapshot stores one row per calendar day with aggregate metrics.
type AnalyticsDailySnapshot struct {
	ID             uint   `gorm:"primaryKey;autoIncrement"`
	Date           string `gorm:"uniqueIndex;not null"`
	TotalUsers     int
	DAU            int
	WAU            int
	PayingUsers    int
	TrialUsers     int
	MRR            int
	ChurnCount     int
	FilesProcessed int
	TokensInput    int
	TokensOutput   int
}

// IPPool is used to keep track of used ip suffixes (101-254)
type IPPool struct {
	mu   sync.Mutex
	used map[int]bool
}

func NewIPPool() *IPPool {
	return &IPPool{used: make(map[int]bool)}
}

func (p *IPPool) MarkUsed(suffix int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.used[suffix] = true
}

func (p *IPPool) Acquire() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := 101; i <= 254; i++ {
		if !p.used[i] {
			p.used[i] = true
			return i, nil
		}
	}
	return 0, errors.New("no free IPs")
}

func (p *IPPool) Release(suffix int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.used, suffix)
}
