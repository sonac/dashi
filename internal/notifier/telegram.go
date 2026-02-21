package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Telegram struct {
	Token  string
	ChatID string
	HTTP   *http.Client
}

func NewTelegram(token, chatID string) *Telegram {
	return &Telegram{
		Token:  token,
		ChatID: chatID,
		HTTP:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *Telegram) Enabled() bool {
	return t.Token != "" && t.ChatID != ""
}

func (t *Telegram) Update(token, chatID string) {
	t.Token = token
	t.ChatID = chatID
}

func (t *Telegram) Send(ctx context.Context, msg string) error {
	if !t.Enabled() {
		return fmt.Errorf("telegram not configured")
	}
	payload := map[string]any{"chat_id": t.ChatID, "text": msg, "disable_web_page_preview": true}
	b, _ := json.Marshal(payload)
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := t.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	resp, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	if res.StatusCode >= 300 {
		return fmt.Errorf("telegram status %d: %s", res.StatusCode, string(resp))
	}
	return nil
}
