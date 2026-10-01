// Package api talks to the GuildLink bot's HTTP API.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	Version string
}

func New(baseURL, token, version string) *Client {
	return &Client{BaseURL: baseURL, Token: token, Version: version, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

type Me struct {
	DiscordUserID string `json:"discordUserId"`
	Characters    []struct {
		Name  string `json:"name"`
		Realm string `json:"realm"`
		Level *int   `json:"level"`
	} `json:"characters"`
}

type SyncResult struct {
	Accepted     int `json:"accepted"`
	SkippedStale int `json:"skippedStale"`
	Rejected     []struct {
		Name   string `json:"name"`
		Realm  string `json:"realm"`
		Reason string `json:"reason"`
	} `json:"rejected"`
	RecipesStored    int `json:"recipesStored"`
	InstancesUpdated int `json:"instancesUpdated"`
}

// Error is a non-2xx answer; Message is the server's explanation when it gave one.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("server said: %s (HTTP %d)", e.Message, e.Status)
	}
	return fmt.Sprintf("server returned HTTP %d", e.Status)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/api/v1"+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "GuildLink-Companion/"+c.Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		return &Error{Status: resp.StatusCode, Message: e.Error}
	}
	return json.Unmarshal(data, out)
}

func (c *Client) Me(ctx context.Context) (*Me, error) {
	var me Me
	return &me, c.do(ctx, http.MethodGet, "/me", nil, &me)
}

func (c *Client) Sync(ctx context.Context, payload any) (*SyncResult, error) {
	var res SyncResult
	return &res, c.do(ctx, http.MethodPost, "/sync", payload, &res)
}
