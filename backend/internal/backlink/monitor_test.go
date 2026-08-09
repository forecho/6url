package backlink

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	_ "github.com/mattn/go-sqlite3"

	"github.com/forecho/6url/backend/ent/enttest"
	"github.com/forecho/6url/backend/ent/placement"
	"github.com/forecho/6url/backend/ent/site"
	"github.com/forecho/6url/backend/ent/submission"
)

type fixedChecker struct{ result Result }

func (f fixedChecker) Check(context.Context, string, string) Result { return f.result }

func TestWorkerProcessesDueExchangePlacement(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:backlink-worker?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	application := client.Submission.Create().SetPublicTokenHash("worker-token").SetType(submission.TypeExchange).SetStatus(submission.StatusPublished).SetApplicantEmail("owner@example.com").SetApplicantName("Owner").SetPreferredLocale("en").SetRawName("Example").SetRawURL("https://example.com").SetNormalizedURL("https://example.com").SetNormalizedRegistrableDomain("example.com").SetRawDescription("Example").SetBacklinkURL("https://example.com/links").SaveX(ctx)
	siteRow := client.Site.Create().SetCanonicalURL("https://example.com").SetCanonicalURLHash("worker-site-hash").SetRegistrableDomain("example.com").SetSlug("example").SetStatus(site.StatusPublished).SetCreatedVia(site.CreatedViaSubmission).SaveX(ctx)
	placementRow := client.Placement.Create().SetSiteID(siteRow.ID).SetKind(placement.KindExchange).SetOrigin(placement.OriginSubmission).SetSubmissionID(application.ID).SetStatus(placement.StatusActive).SetOutboundURL("https://example.com").SetBacklinkURL("https://example.com/links").SaveX(ctx)
	now := time.Now().UTC()
	checker := fixedChecker{result: Result{Status: "passed", RequestedURL: "https://example.com/links", FinalURL: "https://example.com/links", MatchedHref: "https://6url.net", CheckedAt: now}}
	worker := Worker{Monitor: Monitor{Client: client, Checker: checker, TargetURL: "https://6url.net"}, Now: func() time.Time { return now }}
	if processed := worker.ProcessDue(ctx); processed != 1 {
		t.Fatalf("processed=%d", processed)
	}
	updated := client.Placement.GetX(ctx, placementRow.ID)
	if updated.LastCheckStatus == nil || *updated.LastCheckStatus != "passed" || client.BacklinkCheck.Query().CountX(ctx) != 1 {
		t.Fatalf("worker did not persist the check: %#v", updated)
	}
}
