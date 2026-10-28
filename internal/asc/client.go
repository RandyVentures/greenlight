package asc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://api.appstoreconnect.apple.com/v1"

type Client struct {
	keyID              string
	issuerID           string
	keyPath            string
	httpClient         *http.Client
	token              string
	tokenExp           time.Time
	scopedApp          string
	scopedVersion      *AppStoreVersion
	scopedBuild        *Build
	scopedBuildFetched bool
}

func NewClient(keyID, issuerID, privateKeyPath string) (*Client, error) {
	c := &Client{
		keyID:    keyID,
		issuerID: issuerID,
		keyPath:  privateKeyPath,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Validate credentials by generating a token
	if err := c.refreshToken(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Client) refreshToken() error {
	token, err := generateToken(c.keyID, c.issuerID, c.keyPath)
	if err != nil {
		return err
	}
	c.token = token
	c.tokenExp = time.Now().Add(15 * time.Minute) // refresh before 20min expiry
	return nil
}

func (c *Client) get(path string, result interface{}) error {
	if time.Now().After(c.tokenExp) {
		if err := c.refreshToken(); err != nil {
			return err
		}
	}

	target := baseURL + path
	if strings.HasPrefix(path, "https://") {
		u, err := url.Parse(path)
		if err != nil || u.Scheme != "https" || u.Host != "api.appstoreconnect.apple.com" || u.User != nil || !strings.HasPrefix(u.Path, "/v1/") {
			return fmt.Errorf("refusing untrusted pagination URL")
		}
		target = path
	}
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	if result != nil {
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// getAll follows Apple's pagination so later locales/assets are not silently skipped.
func getAll[T any](c *Client, path string) ([]T, error) {
	items := []T{}
	seen := map[string]bool{}
	for path != "" {
		if seen[path] || len(seen) >= 1000 {
			return nil, fmt.Errorf("invalid or excessive pagination")
		}
		seen[path] = true
		var page struct {
			Data  []T `json:"data"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}
		if err := c.get(path, &page); err != nil {
			return nil, err
		}
		if page.Data == nil {
			return nil, fmt.Errorf("API list response is missing its data array")
		}
		items = append(items, page.Data...)
		path = page.Links.Next
	}
	return items, nil
}
