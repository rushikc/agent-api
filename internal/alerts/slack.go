package alerts

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

type Slack struct {
	Webhook string
}

func (s Slack) Name() string { return "slack" }

func (s Slack) Notify(n Notification) error {
	if s.Webhook == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]string{"text": n.Title + ": " + n.Message})
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(s.Webhook, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
