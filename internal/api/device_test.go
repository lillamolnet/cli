package api

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestDeviceFixturesFollowSpec(t *testing.T) {
	assertFollowsSpecFor(t, http.MethodPost, "/api/v1/device/codes", 200, readFixture(t, "device_codes.json"))
	assertFollowsSpecFor(t, http.MethodPost, "/api/v1/device/token", 200, readFixture(t, "device_token.json"))
}

func TestStartDeviceLogin(t *testing.T) {
	client, req := server(t, 200, string(readFixture(t, "device_codes.json")))
	client.token = ""

	code, err := client.StartDeviceLogin(context.Background(), "laptop")
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != http.MethodPost || req.URL.Path != "/api/v1/device/codes" || req.Header.Get("Authorization") != "" {
		t.Errorf("request = %s %s, Authorization %q", req.Method, req.URL.Path, req.Header.Get("Authorization"))
	}
	if code.UserCode != "ZPQR-FDMQ" || code.Interval != 5*time.Second || code.ExpiresIn != 10*time.Minute ||
		code.VerificationURIComplete != "https://lillamolnet.se/device?code=ZPQR-FDMQ" {
		t.Errorf("code = %+v", code)
	}
}

func TestPollDeviceToken(t *testing.T) {
	client, _ := server(t, 200, string(readFixture(t, "device_token.json")))

	token, err := client.PollDeviceToken(context.Background(), "lmdc_x")
	if err != nil {
		t.Fatal(err)
	}
	if token.Name != "lillamolnet CLI on laptop" || len(token.Scopes) != 2 || token.ExpiresAt.IsZero() || token.APIKey == "" {
		t.Errorf("token = %+v", token)
	}
}

func TestPollDeviceTokenErrors(t *testing.T) {
	for fixture, want := range map[string]string{
		"device_pending.json":   DeviceAuthorizationPending,
		"device_slow_down.json": DeviceSlowDown,
		"device_expired.json":   DeviceExpiredToken,
	} {
		client, _ := server(t, 400, string(readFixture(t, fixture)))
		_, err := client.PollDeviceToken(context.Background(), "lmdc_x")
		if got := DeviceErrorCode(err); got != want {
			t.Errorf("%s: code = %q, want %q (err %v)", fixture, got, want, err)
		}
	}
}

func TestRevokeCurrentKey(t *testing.T) {
	client, req := server(t, 204, "")

	if err := client.RevokeCurrentKey(context.Background()); err != nil {
		t.Fatal(err)
	}
	if req.Method != http.MethodDelete || req.URL.Path != "/api/v1/me/api_key" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}
