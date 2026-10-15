package api

import (
	"context"
	"testing"
	"time"
)

func TestAppsFixtureFollowsSpec(t *testing.T) {
	for _, fixture := range []string{"apps.json", "apps_for_status.json"} {
		assertFollowsSpec(t, "/api/v1/apps", readFixture(t, fixture))
	}
}

func TestApps(t *testing.T) {
	client, req := server(t, 200, string(readFixture(t, "apps.json")))

	apps, err := client.Apps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/api/v1/apps" {
		t.Errorf("path = %q", req.URL.Path)
	}
	if len(apps) != 2 {
		t.Fatalf("got %d apps", len(apps))
	}

	bakery, shop := apps[0], apps[1]

	if bakery.Name != "Bakery" || bakery.Subdomain != "bakery" || !bakery.AutoDeployEnabled ||
		bakery.AutoDeployBranch != "main" || bakery.StoppedAt != nil {
		t.Errorf("bakery = %+v", bakery)
	}
	if want := time.Date(2026, 9, 27, 13, 27, 44, 0, time.UTC); !bakery.InsertedAt.Equal(want) {
		t.Errorf("InsertedAt = %v, want %v", bakery.InsertedAt, want)
	}

	if shop.Description != "" || shop.DefaultBranch != "" || shop.StoppedBy != "user" {
		t.Errorf("shop = %+v", shop)
	}
	if shop.StoppedAt == nil || !shop.StoppedAt.Equal(time.Date(2026, 10, 5, 12, 30, 0, 0, time.UTC)) {
		t.Errorf("StoppedAt = %v", shop.StoppedAt)
	}

	wantBakery := Settings{ContainerPort: 4000, HealthCheckType: "http", HealthCheckPath: "/health",
		MemoryLimitMB: 512, CPULimit: 1, PidsLimit: 256}
	if bakery.Settings != wantBakery {
		t.Errorf("bakery.Settings = %+v", bakery.Settings)
	}
	wantShop := Settings{ContainerPort: 4000, HealthCheckType: "tcp", HealthCheckPort: 5000,
		MemoryLimitMB: 1024, CPULimit: 0.5, PidsLimit: 256}
	if shop.Settings != wantShop {
		t.Errorf("shop.Settings = %+v", shop.Settings)
	}
}

// An API from before October 2026 has no settings, which leaves them zero.
func TestAppWithoutSettings(t *testing.T) {
	client, _ := server(t, 200, `{"data":[{"id":"a","type":"app","attributes":{"name":"Old","subdomain":"old",
		"repo_url":"https://github.com/a/old","auto_deploy_enabled":false,
		"inserted_at":"2026-09-27T13:27:44Z","updated_at":"2026-09-27T13:27:44Z"}}]}`)

	apps, err := client.Apps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if apps[0].Settings != (Settings{}) {
		t.Errorf("Settings = %+v", apps[0].Settings)
	}
}

func TestAPITimeFormats(t *testing.T) {
	cases := map[string]time.Time{
		`"2026-09-27T13:27:44"`:        time.Date(2026, 9, 27, 13, 27, 44, 0, time.UTC),
		`"2026-09-27T13:27:44Z"`:       time.Date(2026, 9, 27, 13, 27, 44, 0, time.UTC),
		`"2026-09-27T15:27:44+02:00"`:  time.Date(2026, 9, 27, 13, 27, 44, 0, time.UTC),
		`"2026-09-27T13:27:44.123456"`: time.Date(2026, 9, 27, 13, 27, 44, 123456000, time.UTC),
	}

	for input, want := range cases {
		var got apiTime
		if err := got.UnmarshalJSON([]byte(input)); err != nil {
			t.Errorf("%s: %v", input, err)
			continue
		}
		if !got.Equal(want) {
			t.Errorf("%s: got %v, want %v", input, got.Time, want)
		}
	}

	var bad apiTime
	if err := bad.UnmarshalJSON([]byte(`"yesterday"`)); err == nil {
		t.Error("expected an error for an unknown format")
	}
}
