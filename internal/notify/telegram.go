package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"git.juzo.io/juzo/hashi/internal/secrets"
)

// TypeTelegram is the Telegram bot provider (testdata/contracts/telegram, Bot API).
const TypeTelegram = "telegram"

// TelegramAPI is the Bot API's base address.
const TelegramAPI = "https://api.telegram.org"

// TelegramConfig is everything but the bot token.
type TelegramConfig struct {
	ChatID string `json:"chat_id"`
}

// Telegram talks to one bot.
type Telegram struct {
	Token  secrets.Value
	ChatID string
	Base   string // TelegramAPI unless a test says otherwise
	Client *http.Client
}

type telegramReply struct {
	OK          bool            `json:"ok"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

// call does one Bot API method. The token is part of the URL, so a transport error drops the URL (invariant 5).
func (t Telegram) call(ctx context.Context, method string, body any) (json.RawMessage, error) {
	base := t.Base
	if base == "" {
		base = TelegramAPI
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/bot"+t.Token.Reveal()+"/"+method, bytes.NewReader(b))
	if err != nil {
		return nil, errors.New("telegram: could not build the request")
	}
	req.Header.Set("Content-Type", "application/json")
	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("telegram: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}
	var r telegramReply
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("telegram: unexpected answer (HTTP %d)", res.StatusCode)
	}
	if !r.OK {
		return nil, fmt.Errorf("telegram: %s", r.Description)
	}
	return r.Result, nil
}

// Send implements Sender.
func (t Telegram) Send(ctx context.Context, m Message) error {
	_, err := t.call(ctx, "sendMessage", map[string]any{
		"chat_id": t.ChatID, "text": telegramHTML(m), "parse_mode": "HTML",
		"link_preview_options": map[string]bool{"is_disabled": true},
	})
	return err
}

// TelegramChat is a chat a bot has seen.
type TelegramChat struct {
	ID, Title string
}

// DiscoverChats reads the bot's recent messages (07 §6): the admin messages the bot, Hashi finds the chat. Newest first.
func (t Telegram) DiscoverChats(ctx context.Context) ([]TelegramChat, error) {
	raw, err := t.call(ctx, "getUpdates", map[string]int{"limit": 100, "timeout": 0})
	if err != nil {
		return nil, err
	}
	var updates []struct {
		Message *struct {
			Chat struct {
				ID        int64  `json:"id"`
				Title     string `json:"title"`
				FirstName string `json:"first_name"`
				Username  string `json:"username"`
			} `json:"chat"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &updates); err != nil {
		return nil, fmt.Errorf("telegram: updates: %w", err)
	}
	var out []TelegramChat
	for _, u := range slices.Backward(updates) {
		if u.Message == nil {
			continue
		}
		c := u.Message.Chat
		id := strconv.FormatInt(c.ID, 10)
		if slices.ContainsFunc(out, func(x TelegramChat) bool { return x.ID == id }) {
			continue
		}
		title := strings.TrimSpace(c.Title)
		if title == "" {
			title = strings.TrimSpace(c.FirstName)
		}
		if title == "" {
			title = c.Username
		}
		out = append(out, TelegramChat{ID: id, Title: title})
	}
	return out, nil
}

// Validate checks the chat ID's shape: a number or a @channel name.
func (c TelegramConfig) Validate() error {
	if strings.HasPrefix(c.ChatID, "@") && len(c.ChatID) > 1 {
		return nil
	}
	if _, err := strconv.ParseInt(c.ChatID, 10, 64); err != nil {
		return fmt.Errorf("%w: the chat must be a number or a @channel name", ErrInvalid)
	}
	return nil
}
