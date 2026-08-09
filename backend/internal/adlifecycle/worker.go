package adlifecycle

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/ad"
	"github.com/forecho/6url/backend/ent/notificationdelivery"
	"github.com/forecho/6url/backend/internal/securepayload"
)

const (
	defaultInterval = 10 * time.Minute
	defaultBatch    = 100
)

type Worker struct {
	Client         *ent.Client
	Sealer         *securepayload.Sealer
	SlackChannelID string
	Interval       time.Duration
	Batch          int
	Now            func() time.Time
}

type notificationPayload struct {
	ApplicantName string     `json:"applicant_name,omitempty"`
	Email         string     `json:"email,omitempty"`
	ResourceID    int        `json:"resource_id"`
	ResourceType  string     `json:"resource_type"`
	Name          string     `json:"name,omitempty"`
	URL           string     `json:"url,omitempty"`
	Status        string     `json:"status"`
	PageURL       string     `json:"page_url,omitempty"`
	StartsAt      *time.Time `json:"starts_at,omitempty"`
	EndsAt        *time.Time `json:"ends_at,omitempty"`
}

func (w Worker) Run(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	w.ProcessDue(ctx)
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
	if w.Client == nil {
		return 0
	}
	now := w.now().UTC()
	batch := w.Batch
	if batch <= 0 {
		batch = defaultBatch
	}
	ids, err := w.Client.Ad.Query().Where(ad.StatusEQ(ad.StatusActive), ad.EndsAtNotNil(), ad.EndsAtLTE(now)).Order(ent.Asc(ad.FieldEndsAt), ent.Asc(ad.FieldID)).Limit(batch).IDs(ctx)
	if err != nil {
		log.Printf("advertisement expiry query failed: %v", err)
		return 0
	}
	processed := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		if w.expireOne(ctx, id, now) {
			processed++
		}
	}
	return processed
}

func (w Worker) expireOne(ctx context.Context, id int, now time.Time) bool {
	tx, err := w.Client.Tx(ctx)
	if err != nil {
		log.Printf("advertisement expiry transaction failed: id=%d error=%v", id, err)
		return false
	}
	defer func() { _ = tx.Rollback() }()
	row, err := tx.Ad.Get(ctx, id)
	if err != nil || row.Status != ad.StatusActive || row.EndsAt == nil || row.EndsAt.After(now) {
		return false
	}
	changed, err := tx.Ad.Update().Where(ad.IDEQ(row.ID), ad.StatusEQ(ad.StatusActive), ad.EndsAtLTE(now), ad.VersionEQ(row.Version)).SetStatus(ad.StatusExpired).AddVersion(1).Save(ctx)
	if err != nil || changed != 1 {
		return false
	}
	keyTime := strconv.FormatInt(now.UnixNano(), 36)
	if _, err := tx.RevalidationJob.Create().SetDedupeKey("ad:expired:" + strconv.Itoa(row.ID) + ":" + keyTime).SetPaths([]string{"/", "/en", "/ja", "/ko"}).Save(ctx); err != nil {
		return false
	}
	if _, err := tx.AdminAuditLog.Create().SetAction("ad.expire").SetResourceType("ad").SetResourceID(strconv.Itoa(row.ID)).SetReason("advertisement display period ended").Save(ctx); err != nil {
		return false
	}
	if row.Origin == ad.OriginAdvertiser && row.AdApplicationID != nil {
		if w.Sealer == nil {
			return false
		}
		application, err := tx.AdApplication.Get(ctx, *row.AdApplicationID)
		if err != nil {
			return false
		}
		payload := notificationPayload{ApplicantName: application.ApplicantName, Email: application.ApplicantEmail, ResourceID: application.ID, ResourceType: "ad_application", Name: application.RawTitle, URL: application.TargetURL, Status: "expired", StartsAt: row.StartsAt, EndsAt: row.EndsAt}
		baseKey := "ad:expired:" + strconv.Itoa(row.ID) + ":" + keyTime
		if err := createDelivery(ctx, tx, *w.Sealer, baseKey+":email", "email", application.ApplicantEmail, application.PreferredLocale, payload, now); err != nil {
			return false
		}
		if w.SlackChannelID != "" && createDelivery(ctx, tx, *w.Sealer, baseKey+":slack", "slack", w.SlackChannelID, "en", payload, now) != nil {
			return false
		}
	} else if w.SlackChannelID != "" {
		if w.Sealer == nil {
			return false
		}
		payload := notificationPayload{ApplicantName: "6URL admin", ResourceID: row.ID, ResourceType: "ad", Name: row.TargetURL, URL: row.TargetURL, Status: "expired", StartsAt: row.StartsAt, EndsAt: row.EndsAt}
		baseKey := "ad:expired:" + strconv.Itoa(row.ID) + ":" + keyTime
		if createDelivery(ctx, tx, *w.Sealer, baseKey+":slack", "slack", w.SlackChannelID, "en", payload, now) != nil {
			return false
		}
	}
	if err := tx.Commit(); err != nil {
		log.Printf("advertisement expiry commit failed: id=%d error=%v", id, err)
		return false
	}
	return true
}

func createDelivery(ctx context.Context, tx *ent.Tx, sealer securepayload.Sealer, key, channel, recipient, locale string, payload notificationPayload, now time.Time) error {
	encrypted, err := sealer.Seal(payload, key)
	if err != nil {
		return err
	}
	_, err = tx.NotificationDelivery.Create().SetDedupeKey(key).SetChannel(notificationdelivery.Channel(channel)).SetKind("ad_application.expired").SetRecipient(recipient).SetLocale(locale).SetTemplateKey("ad_application.expired").SetPayloadEncrypted(encrypted).SetPayloadExpiresAt(now.Add(7 * 24 * time.Hour)).Save(ctx)
	return err
}

func (w Worker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}
