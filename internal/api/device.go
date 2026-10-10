package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// DeviceCode starts a login from the CLI (RFC 8628).
type DeviceCode struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               time.Duration
	Interval                time.Duration
}

// DeviceToken is the API key the login gives, once.
type DeviceToken struct {
	APIKey    string
	Name      string
	Scopes    []string
	ExpiresAt time.Time
}

// The errors from polling for the key, as in RFC 8628.
const (
	DeviceAuthorizationPending = "authorization_pending"
	DeviceSlowDown             = "slow_down"
	DeviceAccessDenied         = "access_denied"
	DeviceExpiredToken         = "expired_token"
	DeviceInvalidGrant         = "invalid_grant"
)

// StartDeviceLogin asks for a code to approve. It needs no API key.
func (c *Client) StartDeviceLogin(ctx context.Context, clientName string) (DeviceCode, error) {
	body, err := json.Marshal(map[string]string{"client_name": clientName})
	if err != nil {
		return DeviceCode{}, err
	}

	var doc document[struct{}, struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
	}]
	if err := c.document(ctx, request{method: http.MethodPost, path: []string{"device", "codes"}, body: body}, &doc); err != nil {
		return DeviceCode{}, err
	}

	m := doc.Meta
	return DeviceCode{
		DeviceCode:              m.DeviceCode,
		UserCode:                m.UserCode,
		VerificationURI:         m.VerificationURI,
		VerificationURIComplete: m.VerificationURIComplete,
		ExpiresIn:               time.Duration(m.ExpiresIn) * time.Second,
		Interval:                time.Duration(m.Interval) * time.Second,
	}, nil
}

// PollDeviceToken asks for the key. Until the code is approved, the error is
// an *Error whose Code is DeviceAuthorizationPending or DeviceSlowDown.
func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string) (DeviceToken, error) {
	body, err := json.Marshal(map[string]string{"device_code": deviceCode})
	if err != nil {
		return DeviceToken{}, err
	}

	var doc document[struct{}, struct {
		APIKey    string   `json:"api_key"`
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		ExpiresAt apiTime  `json:"expires_at"`
	}]
	if err := c.document(ctx, request{method: http.MethodPost, path: []string{"device", "token"}, body: body}, &doc); err != nil {
		return DeviceToken{}, err
	}
	if doc.Meta.APIKey == "" {
		return DeviceToken{}, errors.New("unexpected response from the API: no api_key")
	}
	return DeviceToken{APIKey: doc.Meta.APIKey, Name: doc.Meta.Name, Scopes: doc.Meta.Scopes, ExpiresAt: doc.Meta.ExpiresAt.Time}, nil
}

// RevokeCurrentKey revokes the API key the client uses.
func (c *Client) RevokeCurrentKey(ctx context.Context) error {
	_, err := c.fetch(ctx, request{method: http.MethodDelete, path: []string{"me", "api_key"}, accept: mediaType, limit: maxDocumentSize})
	return err
}

// DeviceErrorCode is the RFC 8628 code of a polling error, or "".
func DeviceErrorCode(err error) string {
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusBadRequest {
		return apiErr.Code
	}
	return ""
}
