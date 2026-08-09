package adlifecycle

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	_ "github.com/mattn/go-sqlite3"

	"github.com/forecho/6url/backend/ent/ad"
	"github.com/forecho/6url/backend/ent/adapplication"
	"github.com/forecho/6url/backend/ent/enttest"
	"github.com/forecho/6url/backend/internal/securepayload"
)

func TestWorkerExpiresAdvertiserAdAtomically(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:ad-expiry?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	assetRow := client.Asset.Create().SetProvider("imgant").SetAlbum("6url").SetExternalID("expiry-image").SetStorageKey("6url/expiry.webp").SetPublicURL("https://cdn.imgant.com/expiry.webp").SetContentHash("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc").SetMimeType("image/webp").SetSizeBytes(1024).SetWidth(1200).SetHeight(630).SaveX(ctx)
	application := client.AdApplication.Create().SetPublicTokenHash("expiry-token").SetStatus(adapplication.StatusPublished).SetApplicantEmail("advertiser@example.com").SetApplicantName("Advertiser").SetPreferredLocale("ja").SetTargetURL("https://example.com").SetRequestedLocales([]string{"ja"}).SetRawTitle("Sponsor").SetRawDescription("Description").SaveX(ctx)
	adRow := client.Ad.Create().SetAdApplicationID(application.ID).SetPaymentOrderID(client.PaymentOrder.Create().SetPurpose("ad").SetAdApplicationID(application.ID).SetAdOfferID(client.AdOffer.Create().SetCode("expiry-offer").SetName("Offer").SetAmountMinor(100).SetDurationDays(1).SaveX(ctx).ID).SetProvider("stripe").SetStatus("paid").SetAmountMinor(100).SetDurationDaysSnapshot(1).SaveX(ctx).ID).SetStatus(ad.StatusActive).SetTargetURL("https://example.com").SetImageAssetID(assetRow.ID).SetStartsAt(now.Add(-25 * time.Hour)).SetEndsAt(now.Add(-time.Hour)).SetOrigin(ad.OriginAdvertiser).SaveX(ctx)
	sealer, _ := securepayload.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	worker := Worker{Client: client, Sealer: &sealer, SlackChannelID: "C123", Now: func() time.Time { return now }}
	if got := worker.ProcessDue(ctx); got != 1 {
		t.Fatalf("processed=%d", got)
	}
	updated := client.Ad.GetX(ctx, adRow.ID)
	if updated.Status != ad.StatusExpired || updated.Version != 2 {
		t.Fatalf("unexpected expired ad: %#v", updated)
	}
	if client.RevalidationJob.Query().CountX(ctx) != 1 || client.NotificationDelivery.Query().CountX(ctx) != 2 || client.AdminAuditLog.Query().CountX(ctx) != 1 {
		t.Fatal("expiry side effects are incomplete")
	}
	if got := worker.ProcessDue(ctx); got != 0 {
		t.Fatalf("second processed=%d", got)
	}
}
