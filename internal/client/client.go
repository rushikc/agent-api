package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/registry"
)

const CallerHeader = "X-Devhub-Caller"

type Client struct {
	HTTP   *http.Client
	Base   string
	Token  string
	Caller string
	Sock   string
}

func Unix(cfg *config.Config, caller string) *Client {
	sock := cfg.SocketPath()
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
	}
	return &Client{
		HTTP:   &http.Client{Transport: tr, Timeout: 30 * time.Second},
		Base:   "http://devhub",
		Caller: caller,
		Sock:   sock,
	}
}

func (c *Client) Healthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/v1/health", nil)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func (c *Client) do(method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.Base+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Caller != "" {
		req.Header.Set(CallerHeader, c.Caller)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s", method, path, bytes.TrimSpace(b))
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

func (c *Client) Sessions(project, kind string) ([]registry.Session, error) {
	q := "/v1/sessions"
	sep := "?"
	if project != "" {
		q += sep + "project=" + project
		sep = "&"
	}
	if kind != "" {
		q += sep + "kind=" + kind
	}
	var out []registry.Session
	err := c.do(http.MethodGet, q, nil, &out)
	return out, err
}

func (c *Client) Create(body map[string]any) (registry.Session, error) {
	var s registry.Session
	err := c.do(http.MethodPost, "/v1/sessions", body, &s)
	return s, err
}

func (c *Client) Restart(id int64) error {
	return c.do(http.MethodPost, fmt.Sprintf("/v1/sessions/%d/restart", id), nil, nil)
}

func (c *Client) Delete(id int64) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/v1/sessions/%d", id), nil, nil)
}

func (c *Client) Logs(id int64, lines int) (string, error) {
	var out struct {
		Logs string `json:"logs"`
	}
	err := c.do(http.MethodGet, fmt.Sprintf("/v1/sessions/%d/logs?lines=%d", id, lines), nil, &out)
	return out.Logs, err
}

func (c *Client) Stats() (json.RawMessage, error) {
	var raw json.RawMessage
	err := c.do(http.MethodGet, "/v1/stats", nil, &raw)
	return raw, err
}

func (c *Client) Ports() (json.RawMessage, error) {
	var raw json.RawMessage
	err := c.do(http.MethodGet, "/v1/ports", nil, &raw)
	return raw, err
}

func (c *Client) FreePort(port int) error {
	return c.do(http.MethodPost, fmt.Sprintf("/v1/ports/%d/free", port), nil, nil)
}

func (c *Client) Orphans() (json.RawMessage, error) {
	var raw json.RawMessage
	err := c.do(http.MethodGet, "/v1/orphans", nil, &raw)
	return raw, err
}

func (c *Client) Skills() (json.RawMessage, error) {
	var raw json.RawMessage
	err := c.do(http.MethodGet, "/v1/skills", nil, &raw)
	return raw, err
}

func (c *Client) Skill(name string) (json.RawMessage, error) {
	var raw json.RawMessage
	err := c.do(http.MethodGet, "/v1/skills/"+name, nil, &raw)
	return raw, err
}

func (c *Client) SyncSkills() error {
	return c.do(http.MethodPost, "/v1/skills/sync", nil, nil)
}

func (c *Client) ProfileUp(project string) (json.RawMessage, error) {
	var raw json.RawMessage
	err := c.do(http.MethodPost, "/v1/profiles/"+project+"/up", map[string]any{"yes": true}, &raw)
	return raw, err
}

func (c *Client) ProfileDown(project string) error {
	return c.do(http.MethodPost, "/v1/profiles/"+project+"/down", nil, nil)
}

func (c *Client) Profiles() (json.RawMessage, error) {
	var raw json.RawMessage
	err := c.do(http.MethodGet, "/v1/profiles", nil, &raw)
	return raw, err
}

func TokenFrom(cfg *config.Config) string {
	b, _ := os.ReadFile(cfg.TokenPath())
	return string(bytes.TrimSpace(b))
}
