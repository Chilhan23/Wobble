package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client interface {
	SendMessage(ctx context.Context, chatID int64, threadID *int64, text string, replyMarkup *InlineKeyboardMarkup) (int64, error)
	SendPhoto(ctx context.Context, chatID int64, threadID *int64, fileReader io.Reader, fileName string, caption string) (int64, error)
	SendVideo(ctx context.Context, chatID int64, threadID *int64, fileReader io.Reader, fileName string, caption string) (int64, error)
	SendDocument(ctx context.Context, chatID int64, threadID *int64, fileReader io.Reader, fileName string, caption string) (int64, error)
	GetFile(ctx context.Context, fileID string) (*File, error)
	DownloadFile(ctx context.Context, filePath string) ([]byte, error)
	EditMessageText(ctx context.Context, chatID int64, messageID int64, text string, replyMarkup *InlineKeyboardMarkup) error
	CreateForumTopic(ctx context.Context, chatID int64, name string) (threadID int64, err error)
	CloseForumTopic(ctx context.Context, chatID int64, threadID int64) error
	DeleteForumTopic(ctx context.Context, chatID int64, threadID int64) error
	AnswerCallbackQuery(ctx context.Context, callbackQueryID string, text string) error
}

type client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(token string, baseURL string) Client {
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	return &client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *client) doRequest(ctx context.Context, method string, payload any) ([]byte, error) {
	targetURL := fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)

	var reqBody []byte
	var err error
	if payload != nil {
		reqBody, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request payload: %w", err)
		}
	}

	for attempt := 0; attempt < 3; attempt++ {
		httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(reqBody))
		if reqErr != nil {
			return nil, fmt.Errorf("failed to create http request: %w", reqErr)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, doErr := c.httpClient.Do(httpReq)
		if doErr != nil {
			var urlErr *url.Error
			if errors.As(doErr, &urlErr) {
				return nil, fmt.Errorf("telegram api request %s failed: %w", method, urlErr.Err)
			}
			return nil, fmt.Errorf("telegram api request %s failed: %w", method, doErr)
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read response body: %w", readErr)
		}

		// Tangani 429 Too Many Requests
		if resp.StatusCode == http.StatusTooManyRequests {
			var apiResp APIResponse[any]
			_ = json.Unmarshal(body, &apiResp)
			retryAfter := 1
			if apiResp.Parameters != nil && apiResp.Parameters.RetryAfter > 0 {
				retryAfter = apiResp.Parameters.RetryAfter
			}
			select {
			case <-time.After(time.Duration(retryAfter) * time.Second):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		return body, nil
	}

	return nil, errors.New("telegram api request exceeded max retries")
}

func truncateTelegramText(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes-3]) + "..."
}

func (c *client) SendMessage(ctx context.Context, chatID int64, threadID *int64, text string, replyMarkup *InlineKeyboardMarkup) (int64, error) {
	text = truncateTelegramText(text, 4000)
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if threadID != nil && *threadID > 0 {
		payload["message_thread_id"] = *threadID
	}
	if replyMarkup != nil {
		payload["reply_markup"] = replyMarkup
	}

	body, err := c.doRequest(ctx, "sendMessage", payload)
	if err != nil {
		return 0, err
	}

	var res APIResponse[Message]
	if err := json.Unmarshal(body, &res); err != nil {
		return 0, fmt.Errorf("failed to unmarshal sendMessage response: %w", err)
	}
	if !res.OK {
		return 0, fmt.Errorf("sendMessage api error: %s", res.Description)
	}

	return res.Result.MessageID, nil
}

func (c *client) sendMedia(ctx context.Context, method string, fieldName string, chatID int64, threadID *int64, fileReader io.Reader, fileName string, caption string) (int64, error) {
	caption = truncateTelegramText(caption, 1024)
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	if err := w.WriteField("chat_id", strconv.FormatInt(chatID, 10)); err != nil {
		return 0, err
	}
	if threadID != nil && *threadID > 0 {
		if err := w.WriteField("message_thread_id", strconv.FormatInt(*threadID, 10)); err != nil {
			return 0, err
		}
	}
	if caption != "" {
		if err := w.WriteField("caption", caption); err != nil {
			return 0, err
		}
		if err := w.WriteField("parse_mode", "HTML"); err != nil {
			return 0, err
		}
	}
	if fileReader != nil {
		fw, err := w.CreateFormFile(fieldName, fileName)
		if err != nil {
			return 0, err
		}
		if _, err := io.Copy(fw, fileReader); err != nil {
			return 0, err
		}
	}
	if err := w.Close(); err != nil {
		return 0, err
	}

	targetURL := fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, &b)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return 0, fmt.Errorf("telegram api request %s failed: %w", method, urlErr.Err)
		}
		return 0, fmt.Errorf("telegram api request %s failed: %w", method, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %w", err)
	}

	var res APIResponse[Message]
	if err := json.Unmarshal(body, &res); err != nil {
		return 0, fmt.Errorf("failed to unmarshal %s response: %w", method, err)
	}
	if !res.OK {
		return 0, fmt.Errorf("%s api error: %s", method, res.Description)
	}

	return res.Result.MessageID, nil
}

func (c *client) SendPhoto(ctx context.Context, chatID int64, threadID *int64, fileReader io.Reader, fileName string, caption string) (int64, error) {
	return c.sendMedia(ctx, "sendPhoto", "photo", chatID, threadID, fileReader, fileName, caption)
}

func (c *client) SendVideo(ctx context.Context, chatID int64, threadID *int64, fileReader io.Reader, fileName string, caption string) (int64, error) {
	return c.sendMedia(ctx, "sendVideo", "video", chatID, threadID, fileReader, fileName, caption)
}

func (c *client) SendDocument(ctx context.Context, chatID int64, threadID *int64, fileReader io.Reader, fileName string, caption string) (int64, error) {
	return c.sendMedia(ctx, "sendDocument", "document", chatID, threadID, fileReader, fileName, caption)
}

func (c *client) GetFile(ctx context.Context, fileID string) (*File, error) {
	payload := map[string]any{"file_id": fileID}
	body, err := c.doRequest(ctx, "getFile", payload)
	if err != nil {
		return nil, err
	}

	var res APIResponse[File]
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("failed to unmarshal getFile response: %w", err)
	}
	if !res.OK {
		return nil, fmt.Errorf("getFile api error: %s", res.Description)
	}
	return &res.Result, nil
}

func (c *client) DownloadFile(ctx context.Context, filePath string) ([]byte, error) {
	targetURL := fmt.Sprintf("%s/file/bot%s/%s", c.baseURL, c.token, filePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return nil, fmt.Errorf("telegram download failed: %w", urlErr.Err)
		}
		return nil, fmt.Errorf("telegram download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram download returned status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (c *client) EditMessageText(ctx context.Context, chatID int64, messageID int64, text string, replyMarkup *InlineKeyboardMarkup) error {
	text = truncateTelegramText(text, 4000)
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if replyMarkup != nil {
		payload["reply_markup"] = replyMarkup
	}

	body, err := c.doRequest(ctx, "editMessageText", payload)
	if err != nil {
		return err
	}

	var res APIResponse[any]
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("failed to unmarshal editMessageText response: %w", err)
	}
	if !res.OK {
		return fmt.Errorf("editMessageText api error: %s", res.Description)
	}

	return nil
}

func (c *client) CreateForumTopic(ctx context.Context, chatID int64, name string) (int64, error) {
	// Batas panjang nama topik di Telegram maksimal 128 karakter
	if len(name) > 120 {
		name = name[:120] + "..."
	}

	payload := map[string]any{
		"chat_id": chatID,
		"name":    name,
	}

	body, err := c.doRequest(ctx, "createForumTopic", payload)
	if err != nil {
		return 0, err
	}

	var res APIResponse[struct {
		MessageThreadID int64 `json:"message_thread_id"`
	}]
	if err := json.Unmarshal(body, &res); err != nil {
		return 0, fmt.Errorf("failed to unmarshal createForumTopic response: %w", err)
	}
	if !res.OK {
		return 0, fmt.Errorf("createForumTopic api error: %s", res.Description)
	}

	return res.Result.MessageThreadID, nil
}

func (c *client) CloseForumTopic(ctx context.Context, chatID int64, threadID int64) error {
	payload := map[string]any{
		"chat_id":           chatID,
		"message_thread_id": threadID,
	}

	body, err := c.doRequest(ctx, "closeForumTopic", payload)
	if err != nil {
		return err
	}

	var res APIResponse[bool]
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("failed to unmarshal closeForumTopic response: %w", err)
	}
	if !res.OK {
		return fmt.Errorf("closeForumTopic api error: %s", res.Description)
	}

	return nil
}

func (c *client) DeleteForumTopic(ctx context.Context, chatID int64, threadID int64) error {
	payload := map[string]any{
		"chat_id":           chatID,
		"message_thread_id": threadID,
	}

	body, err := c.doRequest(ctx, "deleteForumTopic", payload)
	if err != nil {
		return err
	}

	var res APIResponse[bool]
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("failed to unmarshal deleteForumTopic response: %w", err)
	}
	if !res.OK {
		return fmt.Errorf("deleteForumTopic api error: %s", res.Description)
	}

	return nil
}

func (c *client) AnswerCallbackQuery(ctx context.Context, callbackQueryID string, text string) error {
	payload := map[string]any{
		"callback_query_id": callbackQueryID,
	}
	if text != "" {
		payload["text"] = text
	}

	body, err := c.doRequest(ctx, "answerCallbackQuery", payload)
	if err != nil {
		return err
	}

	var res APIResponse[bool]
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("failed to unmarshal answerCallbackQuery response: %w", err)
	}
	if !res.OK {
		return fmt.Errorf("answerCallbackQuery api error: %s", res.Description)
	}

	return nil
}
