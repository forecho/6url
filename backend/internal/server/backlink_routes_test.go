package server

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"

	"github.com/forecho/6url/backend/ent/enttest"
	"github.com/forecho/6url/backend/ent/placement"
	"github.com/forecho/6url/backend/ent/site"
	"github.com/forecho/6url/backend/ent/submission"
	"github.com/forecho/6url/backend/internal/backlink"
	"github.com/forecho/6url/backend/internal/config"
	"github.com/forecho/6url/backend/internal/securepayload"
)

func TestThreeMissingBacklinkChecksSuspendExchangePlacement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:backlink-suspension?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	application := client.Submission.Create().SetPublicTokenHash("backlink-token").SetType(submission.TypeExchange).SetStatus(submission.StatusPublished).SetApplicantEmail("owner@example.com").SetApplicantName("Owner").SetPreferredLocale("en").SetRawName("Example").SetRawURL("https://example.com").SetNormalizedURL("https://example.com").SetNormalizedRegistrableDomain("example.com").SetRawDescription("Example").SetBacklinkURL("https://example.com/links").SaveX(ctx)
	siteRow := client.Site.Create().SetCanonicalURL("https://example.com").SetCanonicalURLHash("hash-backlink").SetRegistrableDomain("example.com").SetSlug("example").SetStatus(site.StatusPublished).SetCreatedVia(site.CreatedViaSubmission).SaveX(ctx)
	placementRow := client.Placement.Create().SetSiteID(siteRow.ID).SetKind(placement.KindExchange).SetOrigin(placement.OriginSubmission).SetSubmissionID(application.ID).SetStatus(placement.StatusActive).SetOutboundURL("https://example.com").SetBacklinkURL("https://example.com/links").SaveX(ctx)
	sealer, _ := securepayload.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	checker := staticBacklinkChecker{result: backlink.Result{Status: "missing", RequestedURL: "https://example.com/links", FinalURL: "https://example.com/links", ErrorCode: "target_link_missing", CheckedAt: time.Now().UTC()}}
	deps := Dependencies{Config: config.Config{AppBaseURL: "https://6url.net"}, Ent: client, PayloadSealer: &sealer, Backlinks: checker}
	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = httptest.NewRequest("POST", "/", nil)
	for attempt := 1; attempt <= 3; attempt++ {
		_, updated, err := executePlacementBacklinkCheck(ginContext, deps, placementRow.ID)
		if err != nil {
			t.Fatal(err)
		}
		if updated.ConsecutiveFailures != attempt {
			t.Fatalf("attempt=%d failures=%d", attempt, updated.ConsecutiveFailures)
		}
	}
	updated := client.Placement.GetX(ctx, placementRow.ID)
	if updated.Status != placement.StatusSuspended || updated.SuspendedReason != "backlink_verification_failed" {
		t.Fatalf("placement was not suspended: %#v", updated)
	}
	if client.BacklinkCheck.Query().CountX(ctx) != 3 || client.NotificationDelivery.Query().CountX(ctx) != 1 {
		t.Fatal("backlink history or suspension notification is incomplete")
	}
}
