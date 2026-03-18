package creem

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"getaced.io/src/config"
	"getaced.io/src/structs"
)

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: config.HTTPTimeoutCreem},
	}
}

func (c *Client) CreateCheckout(req structs.CheckoutRequest) (*structs.CheckoutResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.BaseURL+"/v1/checkouts", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.APIKey)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("creem API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var checkout structs.CheckoutResponse
	if err := json.Unmarshal(respBody, &checkout); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &checkout, nil
}

func (c *Client) CreateBillingPortal(customerID string) (*structs.BillingPortalResponse, error) {
	body, err := json.Marshal(map[string]string{"customer_id": customerID})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.BaseURL+"/v1/customers/billing", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.APIKey)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("creem API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var portal structs.BillingPortalResponse
	if err := json.Unmarshal(respBody, &portal); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &portal, nil
}
