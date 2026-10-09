package api

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestMeFixtureFollowsSpec(t *testing.T) {
	assertFollowsSpec(t, "/api/v1/me", readFixture(t, "me.json"))
}

func TestMe(t *testing.T) {
	client, req := server(t, 200, string(readFixture(t, "me.json")))

	user, err := client.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := User{
		ID:    "2e3dd1de-6ec3-4aec-8c17-21a3d96ad63b",
		Email: "ada@example.com",
		APIKey: APIKey{
			Name:      "laptop",
			Scopes:    []string{"read", "deploy"},
			ExpiresAt: time.Date(2027, 1, 7, 20, 10, 44, 0, time.UTC),
		},
	}
	if !reflect.DeepEqual(user, want) {
		t.Errorf("got  %+v\nwant %+v", user, want)
	}
	if req.URL.Path != "/api/v1/me" {
		t.Errorf("path = %q", req.URL.Path)
	}
}
