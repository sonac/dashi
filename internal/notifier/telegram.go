package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type Telegram struct {
	HTTP *http.Client

	mu     sync.RWMutex
	token  string
	chatID string
}

func NewTelegram(token, chatID string) *Telegram {
	return &Telegram{
		token:  token,
		chatID: chatID,
		HTTP:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *Telegram) Enabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.token != "" && t.chatID != ""
}

func (t *Telegram) Update(token, chatID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.token = token
	t.chatID = chatID
}

func (t *Telegram) Send(ctx context.Context, msg string) error {
	t.mu.RLock()
	token, chatID := t.token, t.chatID
	t.mu.RUnlock()
	if token == "" || chatID == "" {
		return fmt.Errorf("telegram not configured")
	}
	payload := map[string]any{"chat_id": chatID, "text": msg, "disable_web_page_preview": true}
	b, _ := json.Marshal(payload)
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
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
