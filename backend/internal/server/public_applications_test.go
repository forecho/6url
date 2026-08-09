package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"

	"github.com/forecho/6url/backend/ent/category"
	"github.com/forecho/6url/backend/ent/enttest"
	"github.com/forecho/6url/backend/internal/config"
	"github.com/forecho/6url/backend/internal/securepayload"
)

type acceptingTurnstile struct{}

func (acceptingTurnstile) Verify(context.Context, string, string) error { return nil }

func TestSubmissionCreationAndStatusSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:submission-flow?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	categoryRow := client.Category.Create().SetSlug("tools").SetStatus(category.StatusPublished).SaveX(ctx)
	sealer, err := securepayload.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{GinMode: gin.TestMode, Environment: "test", AppBaseURL: "https://6url.net", CORSAllowedOrigins: []string{"https://6url.net"}, ApplicationTokenPepper: "01234567890123456789012345678901"}
	router := New(Dependencies{Config: cfg, Ent: client, Turnstile: acceptingTurnstile{}, PayloadSealer: &sealer})
	payload, _ := json.Marshal(map[string]any{"type": "paid", "applicant_email": "owner@example.com", "applicant_name": "Owner", "preferred_locale": "en", "name": "Example", "url": "https://www.example.com/path#fragment", "description": "Useful directory", "category_id": categoryRow.ID, "turnstile_token": "ok"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/submissions", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || strings.Contains(response.Body.String(), "6url_submission_") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	row := client.Submission.Query().OnlyX(ctx)
	if row.NormalizedURL != "https://www.example.com/path" || row.PublicTokenHash == "" {
		t.Fatalf("unexpected submission: %#v", row)
	}
	delivery := client.NotificationDelivery.Query().OnlyX(ctx)
	var notification notificationPayload
	if err := sealer.Open(delivery.PayloadEncrypted, delivery.DedupeKey, &notification); err != nil {
		t.Fatal(err)
	}
	token := strings.TrimPrefix(notification.StatusURL, "https://6url.net/submissions/status#token=")
	exchangeBody, _ := json.Marshal(map[string]string{"token": token})
	exchange := httptest.NewRequest(http.MethodPost, "/api/v1/submission-session", bytes.NewReader(exchangeBody))
	exchange.Header.Set("Content-Type", "application/json")
	exchangeResponse := httptest.NewRecorder()
	router.ServeHTTP(exchangeResponse, exchange)
	if exchangeResponse.Code != http.StatusCreated {
		t.Fatalf("exchange status=%d body=%s", exchangeResponse.Code, exchangeResponse.Body.String())
	}
	cookies := exchangeResponse.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != developmentSubmissionCookie || !cookies[0].HttpOnly {
		t.Fatalf("unexpected cookies: %#v", cookies)
	}
	statusRequest := httptest.NewRequest(http.MethodGet, "/api/v1/submissions/me", nil)
	statusRequest.AddCookie(cookies[0])
	statusResponse := httptest.NewRecorder()
	router.ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK || !strings.Contains(statusResponse.Body.String(), `"status":"received"`) {
		t.Fatalf("status=%d body=%s", statusResponse.Code, statusResponse.Body.String())
	}
}
