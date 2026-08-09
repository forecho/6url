package revalidation

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/revalidationjob"
)

const (
	defaultInterval = 10 * time.Second
	staleAfter      = 10 * time.Minute
	requestTimeout  = 15 * time.Second
)

type Worker struct {
	Client   *ent.Client
	Endpoint string
	Secret   string
	HTTP     *http.Client
	Interval time.Duration
	Batch    int
	Now      func() time.Time
}

type webhookPayload struct {
	Paths     []string `json:"paths"`
	EventID   string   `json:"event_id"`
	Timestamp int64    `json:"timestamp"`
}

func (w Worker) Run(ctx context.Context) {
	w.ProcessDue(ctx)
	interval := w.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	ticker := time.NewTicker(interval)
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

func (w Worker) ProcessDue(ctx context.Context) int {
	if w.Client == nil || strings.TrimSpace(w.Endpoint) == "" || len(w.Secret) < 32 {
		return 0
	}
	now := w.now()
	_, _ = w.Client.RevalidationJob.Update().Where(revalidationjob.StatusEQ(revalidationjob.StatusProcessing), revalidationjob.ClaimedAtLT(now.Add(-staleAfter))).SetStatus(revalidationjob.StatusPending).SetNextRetryAt(now).ClearClaimedAt().Save(ctx)
	batch := w.Batch
	if batch <= 0 {
		batch = 20
	}
	ids, err := w.Client.RevalidationJob.Query().Where(revalidationjob.StatusEQ(revalidationjob.StatusPending), revalidationjob.NextRetryAtLTE(now), revalidationjob.AttemptsLT(5)).Order(ent.Asc(revalidationjob.FieldNextRetryAt), ent.Asc(revalidationjob.FieldID)).Limit(batch).IDs(ctx)
	if err != nil {
		log.Printf("revalidation worker query failed: %v", err)
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

func (w Worker) processOne(ctx context.Context, id int) bool {
	changed, err := w.Client.RevalidationJob.Update().Where(revalidationjob.IDEQ(id), revalidationjob.StatusEQ(revalidationjob.StatusPending)).SetStatus(revalidationjob.StatusProcessing).SetClaimedAt(w.now()).AddAttempts(1).Save(ctx)
	if err != nil || changed != 1 {
		return false
	}
	row, err := w.Client.RevalidationJob.Get(ctx, id)
	if err != nil {
		return false
	}
	payload := webhookPayload{Paths: row.Paths, EventID: strconv.Itoa(row.ID), Timestamp: w.now().Unix()}
	body, err := json.Marshal(payload)
	if err == nil {
		sendCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		err = w.send(sendCtx, payload.Timestamp, body)
		cancel()
	}
	if err == nil {
		_, saveErr := w.Client.RevalidationJob.UpdateOneID(row.ID).SetStatus(revalidationjob.StatusCompleted).SetCompletedAt(w.now()).ClearClaimedAt().SetLastError("").Save(ctx)
		if saveErr != nil {
			log.Printf("revalidation worker mark completed failed: id=%d error=%v", row.ID, saveErr)
		}
		return true
	}
	message := strings.TrimSpace(strings.ReplaceAll(err.Error(), "\n", " "))
	if len(message) > 500 {
		message = message[:500]
	}
	update := w.Client.RevalidationJob.UpdateOneID(row.ID).SetLastError(message).ClearClaimedAt()
	if row.Attempts >= row.MaxAttempts {
		update.SetStatus(revalidationjob.StatusFailed)
	} else {
		update.SetStatus(revalidationjob.StatusPending).SetNextRetryAt(w.now().Add(retryDelay(row.Attempts)))
	}
	if _, saveErr := update.Save(ctx); saveErr != nil {
		log.Printf("revalidation worker retry update failed: id=%d error=%v", row.ID, saveErr)
	}
	return true
}

func (w Worker) send(ctx context.Context, timestamp int64, body []byte) error {
	mac := hmac.New(sha256.New, []byte(w.Secret))
	_, _ = fmt.Fprintf(mac, "%d.", timestamp)
	_, _ = mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, w.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-6URL-Signature", fmt.Sprintf("t=%d,v1=%s", timestamp, signature))
	httpClient := w.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("revalidation endpoint returned %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var result struct {
		Revalidated bool `json:"revalidated"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil || !result.Revalidated {
		return errors.New("revalidation endpoint returned an invalid response")
	}
	return nil
}

func (w Worker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
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
