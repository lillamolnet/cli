package api

import (
	"context"
	"net/http"
)

// The results of stopping and starting an app.
const (
	PowerStopped     = "stopped"
	PowerStarted     = "started"
	PowerRedeploying = "redeploying"
	PowerNotDeployed = "not_deployed"
)

// StopApp stops the app without removing it, and returns it.
func (c *Client) StopApp(ctx context.Context, appID string) (App, string, error) {
	return c.power(ctx, appID, "stop")
}

// StartApp starts a stopped app, and returns it and what happened: started,
// redeploying when its container was gone, or not_deployed.
func (c *Client) StartApp(ctx context.Context, appID string) (App, string, error) {
	return c.power(ctx, appID, "start")
}

func (c *Client) power(ctx context.Context, appID, action string) (App, string, error) {
	var doc document[resource[appAttributes], struct {
		Result string `json:"result"`
	}]
	if err := c.document(ctx, request{method: http.MethodPost, path: []string{"apps", appID, action}}, &doc); err != nil {
		return App{}, "", err
	}
	return toApp(doc.Data), doc.Meta.Result, nil
}
