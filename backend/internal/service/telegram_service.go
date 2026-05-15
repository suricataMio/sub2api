package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	telegramAPIBaseURL = "https://api.telegram.org/bot"
	telegramTimeout    = 10 * time.Second
)

// TelegramService 封装了向 Telegram 机器人发送消息的功能。
type TelegramService struct {
	httpClient *http.Client
}

// NewTelegramService 创建一个新的 TelegramService 实例。
func NewTelegramService() *TelegramService {
	return &TelegramService{
		httpClient: &http.Client{
			Timeout: telegramTimeout,
		},
	}
}

type telegramSendMessageRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

type telegramAPIResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
}

// SendMessage 通过 Telegram Bot API 向指定 chat_id 发送消息。
// token 是 Bot Token，chatID 是目标 Chat ID（群组或用户）。
func (s *TelegramService) SendMessage(ctx context.Context, token, chatID, text string) error {
	if s == nil || s.httpClient == nil {
		return fmt.Errorf("telegram service not initialized")
	}
	if token == "" {
		return fmt.Errorf("telegram bot token is empty")
	}
	if chatID == "" {
		return fmt.Errorf("telegram chat_id is empty")
	}

	payload := telegramSendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram request: %w", err)
	}

	url := telegramAPIBaseURL + token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send telegram message: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var apiResp telegramAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
		}
		return nil
	}
	if !apiResp.OK {
		return fmt.Errorf("telegram API error: %s", apiResp.Description)
	}
	return nil
}
