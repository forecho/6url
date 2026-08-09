package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"

	"github.com/forecho/6url/backend/ent/enttest"
	"github.com/forecho/6url/backend/ent/submission"
	"github.com/forecho/6url/backend/internal/admintoken"
	"github.com/forecho/6url/backend/internal/config"
	"github.com/forecho/6url/backend/internal/securepayload"
)

func TestPaidSubmissionPreapprovalCreatesUSDOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:submission-review?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	pepper := "01234567890123456789012345678901"
	rawAdmin, prefix, err := admintoken.Generate()
	if err != nil {
		t.Fatal(err)
	}
	admin := client.AdminAPIToken.Create().SetName("reviewer").SetPrefix(prefix).SetTokenHash(admintoken.Hash(rawAdmin, pepper)).SetScopes([]string{"submissions:review"}).SaveX(ctx)
	row := client.Submission.Create().SetPublicTokenHash("public-hash").SetType(submission.TypePaid).SetApplicantEmail("owner@example.com").SetApplicantName("Owner").SetPreferredLocale("en").SetRawName("Example").SetRawURL("https://example.com").SetNormalizedURL("https://example.com").SetNormalizedRegistrableDomain("example.com").SetRawDescription("Example").SaveX(ctx)
	client.ListingPlan.Create().SetCode("listing-standard").SetName("Listing").SetAmountMinor(1500).SetCurrency("usd").SetIncludedLocales([]string{"en"}).SetStripePriceID("price_listing").SetActive(true).SaveX(ctx)
	sealer, err := securepayload.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{GinMode: gin.TestMode, Environment: "test", CORSAllowedOrigins: []string{"https://6url.net"}, AdminTokenPepper: pepper}
	router := New(Dependencies{Config: cfg, Ent: client, PayloadSealer: &sealer})
	request := httptest.NewRequest(http.MethodPost, "/admin/v1/submissions/"+strconv.Itoa(row.ID)+"/preapprove", bytes.NewBufferString(`{}`))
	request.Header.Set("Authorization", "Bearer "+rawAdmin)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	updated := client.Submission.GetX(ctx, row.ID)
	if updated.Status != submission.StatusAwaitingPayment || updated.ReviewerTokenID == nil || *updated.ReviewerTokenID != admin.ID {
		t.Fatalf("unexpected submission: %#v", updated)
	}
	order := client.PaymentOrder.Query().OnlyX(ctx)
	if order.AmountMinor != 1500 || order.Currency != "usd" || order.StripePriceIDSnapshot == nil || *order.StripePriceIDSnapshot != "price_listing" {
		t.Fatalf("unexpected order: %#v", order)
	}
	if client.AdminAuditLog.Query().CountX(ctx) != 1 || client.NotificationDelivery.Query().CountX(ctx) != 1 {
		t.Fatal("review side effects are incomplete")
	}
}
