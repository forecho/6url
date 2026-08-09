package backlink

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/forecho/6url/backend/ent"
	entbacklinkcheck "github.com/forecho/6url/backend/ent/backlinkcheck"
	"github.com/forecho/6url/backend/ent/notificationdelivery"
	"github.com/forecho/6url/backend/ent/placement"
	"github.com/forecho/6url/backend/internal/securepayload"
)

type Monitor struct {
	Client         *ent.Client
	Checker        Checker
	TargetURL      string
	Sealer         *securepayload.Sealer
	SlackChannelID string
	Now            func() time.Time
}

var ErrNotExchangePlacement = errors.New("placement is not an exchange placement")

func (m Monitor) CheckPlacement(ctx context.Context, placementID int) (Result, *ent.Placement, error) {
	row, err := m.Client.Placement.Get(ctx, placementID)
	if err != nil {
		return Result{}, nil, err
	}
	if row.Kind != placement.KindExchange || row.BacklinkURL == nil {
		return Result{}, nil, ErrNotExchangePlacement
	}
	if m.Checker == nil {
		return Result{}, nil, errors.New("backlink checker is unavailable")
	}
	result := m.Checker.Check(ctx, *row.BacklinkURL, m.TargetURL)
	tx, err := m.Client.Tx(ctx)
	if err != nil {
		return result, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := saveResult(ctx, tx, row.ID, result); err != nil {
		return result, nil, err
	}
	updater := tx.Placement.UpdateOneID(row.ID).Where(placement.UpdatedAtEQ(row.UpdatedAt)).SetLastCheckStatus(result.Status).SetLastCheckedAt(result.CheckedAt)
	failures := row.ConsecutiveFailures
	suspendedNow := false
	switch result.Status {
	case "passed":
		failures = 0
		updater.SetConsecutiveFailures(0)
		if row.Status == placement.StatusSuspended && row.SuspendedReason == "backlink_verification_failed" {
			updater.SetStatus(placement.StatusActive).ClearSuspendedReason()
		}
	case "missing", "blocked":
		failures++
		updater.SetConsecutiveFailures(failures)
	case "unsafe":
		failures = 3
		updater.SetConsecutiveFailures(failures)
	}
	if failures >= 3 && row.Status == placement.StatusActive {
		updater.SetStatus(placement.StatusSuspended).SetSuspendedReason("backlink_verification_failed")
		suspendedNow = true
	}
	updated, err := updater.Save(ctx)
	if err != nil {
		return result, nil, err
	}
	if suspendedNow && row.SubmissionID != nil {
		if err := m.queueSuspension(ctx, tx, *row.SubmissionID); err != nil {
			return result, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, nil, err
	}
	return result, updated, nil
}

func (m Monitor) queueSuspension(ctx context.Context, tx *ent.Tx, submissionID int) error {
	if m.Sealer == nil {
		return errors.New("notification payload sealer is unavailable")
	}
	application, err := tx.Submission.Get(ctx, submissionID)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"applicant_name":   application.ApplicantName,
		"email":            application.ApplicantEmail,
		"resource_id":      application.ID,
		"resource_type":    "submission",
		"application_type": string(application.Type),
		"name":             application.RawName,
		"url":              application.NormalizedURL,
		"status":           "exchange_suspended",
		"review_note":      "The reciprocal link could not be verified after three consecutive checks.",
	}
	baseKey := "submission:exchange_suspended:" + strconv.Itoa(application.ID) + ":" + strconv.FormatInt(m.now().UnixNano(), 36)
	if err := m.createDelivery(ctx, tx, baseKey+":email", notificationdelivery.ChannelEmail, application.ApplicantEmail, application.PreferredLocale, payload); err != nil {
		return err
	}
	if strings.TrimSpace(m.SlackChannelID) != "" {
		if err := m.createDelivery(ctx, tx, baseKey+":slack", notificationdelivery.ChannelSlack, m.SlackChannelID, "en", payload); err != nil {
			return err
		}
	}
	return nil
}

func (m Monitor) createDelivery(ctx context.Context, tx *ent.Tx, dedupeKey string, channel notificationdelivery.Channel, recipient, localeCode string, payload map[string]any) error {
	encrypted, err := m.Sealer.Seal(payload, dedupeKey)
	if err != nil {
		return err
	}
	_, err = tx.NotificationDelivery.Create().SetDedupeKey(dedupeKey).SetChannel(channel).SetKind("submission.exchange_suspended").SetRecipient(recipient).SetLocale(localeCode).SetTemplateKey("submission.exchange_suspended").SetPayloadEncrypted(encrypted).SetPayloadExpiresAt(m.now().Add(7 * 24 * time.Hour)).Save(ctx)
	return err
}

func saveResult(ctx context.Context, tx *ent.Tx, placementID int, result Result) error {
	creator := tx.BacklinkCheck.Create().SetPlacementID(placementID).SetStatus(entStatus(result.Status)).SetRequestedURL(result.RequestedURL).SetFinalURL(result.FinalURL).SetMatchedHref(result.MatchedHref).SetMatchedAnchor(result.MatchedAnchor).SetMatchedRel(result.MatchedRel).SetRedirectChainJSON(result.RedirectChainJSON()).SetErrorCode(result.ErrorCode).SetCheckedAt(result.CheckedAt)
	if result.HTTPStatus != nil {
		creator.SetHTTPStatus(*result.HTTPStatus)
	}
	_, err := creator.Save(ctx)
	return err
}

func entStatus(value string) entbacklinkcheck.Status {
	switch value {
	case "passed":
		return entbacklinkcheck.StatusPassed
	case "missing":
		return entbacklinkcheck.StatusMissing
	case "blocked":
		return entbacklinkcheck.StatusBlocked
	case "unsafe":
		return entbacklinkcheck.StatusUnsafe
	default:
		return entbacklinkcheck.StatusNetworkError
	}
}

func (m Monitor) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

type Worker struct {
	Monitor  Monitor
	Interval time.Duration
	Batch    int
	Now      func() time.Time
}

func (w Worker) Run(ctx context.Context) {
	w.ProcessDue(ctx)
	interval := w.Interval
	if interval <= 0 {
		interval = time.Hour
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
	if w.Monitor.Client == nil {
		return 0
	}
	now := time.Now().UTC()
	if w.Now != nil {
		now = w.Now().UTC()
	}
	batch := w.Batch
	if batch <= 0 {
		batch = 50
	}
	ids, err := w.Monitor.Client.Placement.Query().Where(
		placement.KindEQ(placement.KindExchange),
		placement.StatusIn(placement.StatusActive, placement.StatusSuspended),
		placement.BacklinkURLNotNil(),
		placement.Or(placement.LastCheckedAtIsNil(), placement.LastCheckedAtLTE(now.Add(-24*time.Hour))),
	).Order(ent.Asc(placement.FieldLastCheckedAt), ent.Asc(placement.FieldID)).Limit(batch).IDs(ctx)
	if err != nil {
		log.Printf("backlink monitor query failed: %v", err)
		return 0
	}
	processed := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		if _, _, err := w.Monitor.CheckPlacement(ctx, id); err != nil {
			log.Printf("backlink monitor check failed: placement_id=%d error=%v", id, err)
			continue
		}
		processed++
	}
	return processed
}
