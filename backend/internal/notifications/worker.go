package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/notificationdelivery"
	"github.com/forecho/6url/backend/internal/config"
	"github.com/forecho/6url/backend/internal/securepayload"
)

const (
	workerInterval  = 10 * time.Second
	workerBatch     = 20
	staleAfter      = 10 * time.Minute
	deliveryTimeout = 20 * time.Second
)

type Payload struct {
	ApplicantName   string     `json:"applicant_name,omitempty"`
	Email           string     `json:"email,omitempty"`
	ResourceID      int        `json:"resource_id"`
	ResourceType    string     `json:"resource_type"`
	ApplicationType string     `json:"application_type,omitempty"`
	Name            string     `json:"name,omitempty"`
	URL             string     `json:"url,omitempty"`
	Status          string     `json:"status"`
	StatusURL       string     `json:"status_url,omitempty"`
	ReviewNote      string     `json:"review_note,omitempty"`
	PageURL         string     `json:"page_url,omitempty"`
	StartsAt        *time.Time `json:"starts_at,omitempty"`
	EndsAt          *time.Time `json:"ends_at,omitempty"`
}

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

type EmailSender interface {
	Send(context.Context, Message) error
}

type Worker struct {
	Client   *ent.Client
	Sealer   securepayload.Sealer
	Email    EmailSender
	Slack    *SlackSender
	Interval time.Duration
	Batch    int
	Now      func() time.Time
}

func NewWorker(client *ent.Client, cfg config.Config, sealer securepayload.Sealer) *Worker {
	httpClient := &http.Client{Timeout: deliveryTimeout}
	return &Worker{
		Client:   client,
		Sealer:   sealer,
		Email:    NewEmailSender(EmailConfig{Provider: cfg.EmailProvider, APIKey: cfg.EmailAPIKey, From: cfg.EmailFrom, BaseURL: cfg.EmailAPIBaseURL}, httpClient),
		Slack:    &SlackSender{Token: cfg.SlackBotToken, HTTP: httpClient},
		Interval: workerInterval,
		Batch:    workerBatch,
		Now:      time.Now,
	}
}

func (w *Worker) Run(ctx context.Context) {
	if w.Interval <= 0 {
		w.Interval = workerInterval
	}
	w.ProcessDue(ctx)
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.ProcessDue(ctx)
		}
	}
}

func (w *Worker) ProcessDue(ctx context.Context) int {
	if w == nil || w.Client == nil {
		return 0
	}
	now := w.now()
	_, _ = w.Client.NotificationDelivery.Update().Where(notificationdelivery.StatusEQ(notificationdelivery.StatusSending), notificationdelivery.ClaimedAtLT(now.Add(-staleAfter))).SetStatus(notificationdelivery.StatusPending).SetNextRetryAt(now).ClearClaimedAt().Save(ctx)
	expired, _ := w.Client.NotificationDelivery.Query().Where(notificationdelivery.StatusEQ(notificationdelivery.StatusPending), notificationdelivery.PayloadExpiresAtLTE(now)).IDs(ctx)
	if len(expired) > 0 {
		_, _ = w.Client.NotificationDelivery.Update().Where(notificationdelivery.IDIn(expired...)).SetStatus(notificationdelivery.StatusFailed).SetPayloadEncrypted("").SetLastError("notification payload expired").Save(ctx)
	}
	batch := w.Batch
	if batch <= 0 {
		batch = workerBatch
	}
	ids, err := w.Client.NotificationDelivery.Query().Where(notificationdelivery.StatusEQ(notificationdelivery.StatusPending), notificationdelivery.NextRetryAtLTE(now), notificationdelivery.AttemptsLT(5)).Order(ent.Asc(notificationdelivery.FieldNextRetryAt), ent.Asc(notificationdelivery.FieldID)).Limit(batch).IDs(ctx)
	if err != nil {
		log.Printf("notification worker query failed: %v", err)
		return 0
	}
	processed := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		if w.processOne(ctx, id) {
			processed++
		}
	}
	return processed
}

func (w *Worker) processOne(ctx context.Context, id int) bool {
	now := w.now()
	changed, err := w.Client.NotificationDelivery.Update().Where(notificationdelivery.IDEQ(id), notificationdelivery.StatusEQ(notificationdelivery.StatusPending)).SetStatus(notificationdelivery.StatusSending).SetClaimedAt(now).AddAttempts(1).Save(ctx)
	if err != nil || changed != 1 {
		return false
	}
	row, err := w.Client.NotificationDelivery.Get(ctx, id)
	if err != nil {
		return false
	}
	var payload Payload
	if err := w.Sealer.Open(row.PayloadEncrypted, row.DedupeKey, &payload); err != nil {
		w.fail(ctx, row, err)
		return true
	}
	sendCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	err = w.deliver(sendCtx, row, payload)
	cancel()
	if err == nil {
		_, saveErr := w.Client.NotificationDelivery.UpdateOneID(row.ID).SetStatus(notificationdelivery.StatusSent).SetSentAt(w.now()).ClearClaimedAt().SetPayloadEncrypted("").SetLastError("").Save(ctx)
		if saveErr != nil {
			log.Printf("notification worker mark sent failed: id=%d error=%v", row.ID, saveErr)
		}
		return true
	}
	w.fail(ctx, row, err)
	return true
}

func (w *Worker) fail(ctx context.Context, row *ent.NotificationDelivery, deliveryErr error) {
	message := strings.TrimSpace(strings.ReplaceAll(deliveryErr.Error(), "\n", " "))
	if len(message) > 500 {
		message = message[:500]
	}
	update := w.Client.NotificationDelivery.UpdateOneID(row.ID).SetLastError(message).ClearClaimedAt()
	if row.Attempts >= row.MaxAttempts {
		update.SetStatus(notificationdelivery.StatusFailed).SetPayloadEncrypted("")
	} else {
		update.SetStatus(notificationdelivery.StatusPending).SetNextRetryAt(w.now().Add(retryDelay(row.Attempts)))
	}
	if _, err := update.Save(ctx); err != nil {
		log.Printf("notification worker retry update failed: id=%d error=%v", row.ID, err)
	}
}

func (w *Worker) deliver(ctx context.Context, row *ent.NotificationDelivery, payload Payload) error {
	subject, textBody, htmlBody := render(row.Locale, row.Kind, payload)
	switch row.Channel {
	case notificationdelivery.ChannelEmail:
		if w.Email == nil {
			return errors.New("email sender is unavailable")
		}
		return w.Email.Send(ctx, Message{To: row.Recipient, Subject: subject, Text: textBody, HTML: htmlBody})
	case notificationdelivery.ChannelSlack:
		if w.Slack == nil {
			return errors.New("Slack sender is unavailable")
		}
		return w.Slack.Send(ctx, row.Recipient, subject+"\n"+textBody)
	default:
		return fmt.Errorf("unsupported notification channel: %s", row.Channel)
	}
}

func (w *Worker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 5 {
		attempt = 5
	}
	return time.Duration(1<<uint(attempt-1)) * time.Minute
}

type EmailConfig struct {
	Provider string
	APIKey   string
	From     string
	BaseURL  string
}

type emailSender struct {
	provider string
	apiKey   string
	from     string
	baseURL  string
	http     *http.Client
}

func NewEmailSender(cfg EmailConfig, client *http.Client) EmailSender {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = "unosend"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		if provider == "sendflare" {
			baseURL = "https://api.sendflare.com"
		} else {
			baseURL = "https://api.unosend.co"
		}
	}
	if client == nil {
		client = &http.Client{Timeout: deliveryTimeout}
	}
	return &emailSender{provider: provider, apiKey: strings.TrimSpace(cfg.APIKey), from: strings.TrimSpace(cfg.From), baseURL: baseURL, http: client}
}

func (s *emailSender) Send(ctx context.Context, message Message) error {
	if s.apiKey == "" || s.from == "" {
		return errors.New("email sender is not configured")
	}
	var endpoint string
	var payload any
	switch s.provider {
	case "unosend":
		endpoint = s.baseURL + "/emails"
		payload = map[string]any{"from": s.from, "to": []string{message.To}, "subject": message.Subject, "text": message.Text, "html": message.HTML}
	case "sendflare":
		endpoint = s.baseURL + "/v1/send"
		payload = map[string]any{"from": s.from, "to": message.To, "subject": message.Subject, "body": message.HTML}
	default:
		return fmt.Errorf("unsupported email provider: %s", s.provider)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("email API returned %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if s.provider == "sendflare" {
		var result struct {
			Code    int  `json:"code"`
			Success bool `json:"success"`
		}
		if json.Unmarshal(responseBody, &result) != nil || !result.Success || result.Code != 0 {
			return errors.New("Sendflare rejected the email")
		}
	}
	return nil
}

type SlackSender struct {
	Token string
	HTTP  *http.Client
}

func (s *SlackSender) Send(ctx context.Context, channel, text string) error {
	if strings.TrimSpace(s.Token) == "" || strings.TrimSpace(channel) == "" {
		return errors.New("Slack sender is not configured")
	}
	body, _ := json.Marshal(map[string]string{"channel": channel, "text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(s.Token))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpClient := s.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: deliveryTimeout}
	}
	response, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&result) != nil || !result.OK {
		return fmt.Errorf("Slack API rejected the message: %s", result.Error)
	}
	return nil
}

type copySet struct {
	Subject       string
	Greeting      string
	StatusLabel   string
	ReviewLabel   string
	StatusLink    string
	ResourceLabel string
	PageLink      string
	StartsLabel   string
	EndsLabel     string
}

var localizedCopy = map[string]copySet{
	"zh-CN": {"6URL 申请状态更新", "您好", "当前状态", "审核说明", "查看申请状态", "申请", "查看展示页面", "开始时间", "结束时间"},
	"en":    {"Your 6URL application update", "Hello", "Current status", "Review note", "View application status", "Application", "View published page", "Starts at", "Ends at"},
	"ja":    {"6URL 申請ステータス更新", "こんにちは", "現在のステータス", "審査コメント", "申請状況を確認", "申請", "掲載ページを見る", "開始日時", "終了日時"},
	"ko":    {"6URL 신청 상태 업데이트", "안녕하세요", "현재 상태", "검토 메모", "신청 상태 보기", "신청", "게시 페이지 보기", "시작 시간", "종료 시간"},
}

func render(localeCode, kind string, payload Payload) (string, string, string) {
	copy := localizedCopy[localeCode]
	if copy.Subject == "" {
		copy = localizedCopy["en"]
	}
	subject := copy.Subject
	if strings.HasSuffix(kind, ".received") {
		subject = strings.TrimSuffix(copy.Subject, " update")
	}
	lines := []string{copy.Greeting + " " + payload.ApplicantName + ",", "", copy.ResourceLabel + ": " + payload.Name, copy.StatusLabel + ": " + payload.Status}
	if payload.ReviewNote != "" {
		lines = append(lines, copy.ReviewLabel+": "+payload.ReviewNote)
	}
	if payload.StatusURL != "" {
		lines = append(lines, "", copy.StatusLink+": "+payload.StatusURL)
	}
	if payload.PageURL != "" {
		lines = append(lines, "", copy.PageLink+": "+payload.PageURL)
	}
	if payload.StartsAt != nil {
		lines = append(lines, copy.StartsLabel+": "+payload.StartsAt.UTC().Format(time.RFC3339))
	}
	if payload.EndsAt != nil {
		lines = append(lines, copy.EndsLabel+": "+payload.EndsAt.UTC().Format(time.RFC3339))
	}
	textBody := strings.Join(lines, "\n")
	htmlLines := []string{"<p>" + html.EscapeString(copy.Greeting+" "+payload.ApplicantName) + ",</p>", "<p><strong>" + html.EscapeString(copy.ResourceLabel) + ":</strong> " + html.EscapeString(payload.Name) + "<br><strong>" + html.EscapeString(copy.StatusLabel) + ":</strong> " + html.EscapeString(payload.Status) + "</p>"}
	if payload.ReviewNote != "" {
		htmlLines = append(htmlLines, "<p><strong>"+html.EscapeString(copy.ReviewLabel)+":</strong> "+html.EscapeString(payload.ReviewNote)+"</p>")
	}
	if payload.StatusURL != "" {
		htmlLines = append(htmlLines, `<p><a href="`+html.EscapeString(payload.StatusURL)+`">`+html.EscapeString(copy.StatusLink)+"</a></p>")
	}
	if payload.PageURL != "" {
		htmlLines = append(htmlLines, `<p><a href="`+html.EscapeString(payload.PageURL)+`">`+html.EscapeString(copy.PageLink)+"</a></p>")
	}
	if payload.StartsAt != nil {
		htmlLines = append(htmlLines, "<p><strong>"+html.EscapeString(copy.StartsLabel)+":</strong> "+html.EscapeString(payload.StartsAt.UTC().Format(time.RFC3339))+"</p>")
	}
	if payload.EndsAt != nil {
		htmlLines = append(htmlLines, "<p><strong>"+html.EscapeString(copy.EndsLabel)+":</strong> "+html.EscapeString(payload.EndsAt.UTC().Format(time.RFC3339))+"</p>")
	}
	return subject, textBody, strings.Join(htmlLines, "")
}
