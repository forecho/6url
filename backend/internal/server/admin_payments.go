package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/ad"
	"github.com/forecho/6url/backend/ent/paymentevent"
	"github.com/forecho/6url/backend/ent/paymentorder"
	"github.com/forecho/6url/backend/ent/placement"
	"github.com/forecho/6url/backend/ent/placementlocale"
	"github.com/forecho/6url/backend/internal/stripeclient"
)

type markPaidRequest struct {
	ExternalRef string `json:"external_ref" binding:"required"`
	Note        string `json:"note" binding:"required"`
}

type refundRequest struct {
	Reason string `json:"reason" binding:"required"`
}

func markManualOrderPaid(c *gin.Context, deps Dependencies) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	key, ok := requireIdempotencyKey(c)
	if !ok {
		return
	}
	var request markPaidRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validTrimmedLength(request.ExternalRef, 255) || !validTrimmedLength(request.Note, 500) {
		writeError(c, http.StatusBadRequest, "invalid_request", "A valid external_ref and audit note are required.")
		return
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to mark the payment order as paid.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	order, err := tx.PaymentOrder.Get(c.Request.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			writeError(c, http.StatusNotFound, "not_found", "The payment order does not exist.")
		} else {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the payment order.")
		}
		return
	}
	externalRef := strings.TrimSpace(request.ExternalRef)
	note := strings.TrimSpace(request.Note)
	eventID := "manual-paid:" + strconv.Itoa(order.ID) + ":" + idempotencyDigest(key)
	eventPayload := "request_sha256:" + stringDigest(externalRef+"\x00"+note)
	if event, err := tx.PaymentEvent.Query().Where(paymentevent.EventIDEQ(eventID)).Only(c.Request.Context()); err == nil {
		if order.Provider != paymentorder.ProviderManual || order.Status != paymentorder.StatusPaid || event.PayloadEncryptedOrRedacted != eventPayload {
			writeError(c, http.StatusConflict, "idempotency_conflict", "Idempotency-Key was already used with different payment data.")
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": order.ID, "status": order.Status, "paid_at": order.PaidAt})
		return
	} else if !ent.IsNotFound(err) {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to verify the manual payment idempotency key.")
		return
	}
	if order.Provider != paymentorder.ProviderManual || order.Status != paymentorder.StatusPendingManual {
		writeError(c, http.StatusConflict, "invalid_state", "Only a pending manual payment order can be marked paid.")
		return
	}
	now := time.Now().UTC()
	changed, err := tx.PaymentOrder.Update().Where(paymentorder.IDEQ(order.ID), paymentorder.StatusEQ(paymentorder.StatusPendingManual), paymentorder.ProviderEQ(paymentorder.ProviderManual)).SetStatus(paymentorder.StatusPaid).SetPaidAt(now).SetExternalRef(externalRef).Save(c.Request.Context())
	if err != nil || changed != 1 {
		writeError(c, http.StatusConflict, "concurrent_update", "The payment order changed while it was being marked paid.")
		return
	}
	order.Status = paymentorder.StatusPaid
	order.PaidAt = &now
	if err := markApplicationPaid(c, deps, tx, order); err != nil {
		writeError(c, http.StatusConflict, "invalid_application_state", "The application is not ready for a manual payment confirmation.")
		return
	}
	if _, err := tx.PaymentEvent.Create().SetProvider("manual").SetEventID(eventID).SetType("admin.order.mark_paid").SetPayloadEncryptedOrRedacted(eventPayload).SetReceivedAt(now).SetProcessedAt(now).Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusConflict, "idempotency_conflict", "This manual payment operation was already recorded.")
		return
	}
	if _, err := tx.AdminAuditLog.Create().SetAdminAPITokenID(adminIdentity(c).ID).SetAction("order.mark_paid").SetResourceType("payment_order").SetResourceID(strconv.Itoa(order.ID)).SetIdempotencyKey(key).SetReason(note).Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to write the payment audit log.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to mark the payment order as paid.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": order.ID, "status": paymentorder.StatusPaid, "paid_at": now})
}

func refundOrder(c *gin.Context, deps Dependencies) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	key, ok := requireIdempotencyKey(c)
	if !ok {
		return
	}
	var request refundRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validTrimmedLength(request.Reason, 500) {
		writeError(c, http.StatusBadRequest, "invalid_request", "A refund reason of at most 500 characters is required.")
		return
	}
	order, err := deps.Ent.PaymentOrder.Get(c.Request.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			writeError(c, http.StatusNotFound, "not_found", "The payment order does not exist.")
		} else {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the payment order.")
		}
		return
	}
	if order.Status == paymentorder.StatusRefunded {
		c.JSON(http.StatusOK, gin.H{"id": order.ID, "status": order.Status, "refunded_at": order.RefundedAt})
		return
	}
	if order.Status == paymentorder.StatusRefundPending {
		c.JSON(http.StatusAccepted, gin.H{"id": order.ID, "status": order.Status, "refund_id": order.ExternalRef})
		return
	}
	if order.Status != paymentorder.StatusPaid || order.Provider != paymentorder.ProviderStripe || order.StripePaymentIntentID == nil {
		writeError(c, http.StatusConflict, "invalid_state", "Only a paid Stripe order with a payment intent can be refunded.")
		return
	}
	providerKey := "6url-refund-" + strconv.Itoa(order.ID) + "-" + idempotencyDigest(key)
	refund, err := deps.StripeRefund.Refund(c.Request.Context(), stripeclient.RefundRequest{OrderID: order.ID, PaymentIntentID: *order.StripePaymentIntentID, AmountMinor: order.AmountMinor, IdempotencyKey: providerKey})
	if err != nil {
		writeError(c, http.StatusBadGateway, "payment_provider_error", "Stripe could not create the refund.")
		return
	}
	if refund.Amount != order.AmountMinor || !strings.EqualFold(refund.Currency, order.Currency) {
		writeError(c, http.StatusBadGateway, "refund_mismatch", "The Stripe refund amount or currency does not match the order.")
		return
	}
	if refund.Status != "succeeded" && refund.Status != "pending" && refund.Status != "requires_action" {
		writeError(c, http.StatusBadGateway, "refund_failed", "Stripe returned an unsuccessful refund status.")
		return
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to store the refund.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	current, err := tx.PaymentOrder.Get(c.Request.Context(), order.ID)
	if err == nil && current.ExternalRef != nil && *current.ExternalRef == refund.ID && current.Status == paymentorder.StatusRefunded {
		c.JSON(http.StatusOK, gin.H{"id": current.ID, "status": current.Status, "refund_id": refund.ID, "refunded_at": current.RefundedAt})
		return
	}
	if err == nil && current.ExternalRef != nil && *current.ExternalRef == refund.ID && current.Status == paymentorder.StatusRefundPending {
		c.JSON(http.StatusAccepted, gin.H{"id": current.ID, "status": current.Status, "refund_id": refund.ID})
		return
	}
	if err != nil || current.Status != paymentorder.StatusPaid {
		writeError(c, http.StatusConflict, "concurrent_update", "The payment order changed while Stripe created the refund.")
		return
	}
	now := time.Now().UTC()
	newStatus := paymentorder.StatusRefundPending
	updater := tx.PaymentOrder.Update().Where(paymentorder.IDEQ(order.ID), paymentorder.StatusEQ(paymentorder.StatusPaid)).SetStatus(newStatus).SetExternalRef(refund.ID)
	if refund.Status == "succeeded" {
		newStatus = paymentorder.StatusRefunded
		updater.SetStatus(newStatus).SetRefundedAt(now)
	}
	changed, err := updater.Save(c.Request.Context())
	if err != nil || changed != 1 {
		writeError(c, http.StatusConflict, "concurrent_update", "The payment order changed while the refund was stored.")
		return
	}
	details, _ := json.Marshal(map[string]any{"refund_id": refund.ID, "status": refund.Status, "amount_minor": refund.Amount, "currency": refund.Currency})
	if _, err := tx.PaymentEvent.Create().SetProvider("stripe").SetEventID("admin-refund:" + refund.ID).SetType("admin.order.refund").SetPayloadEncryptedOrRedacted(string(details)).SetReceivedAt(now).SetProcessedAt(now).Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusConflict, "idempotency_conflict", "This Stripe refund was already recorded.")
		return
	}
	if refund.Status == "succeeded" {
		current.Status = paymentorder.StatusRefunded
		current.RefundedAt = &now
		if err := applySuccessfulRefund(c, deps, tx, current, now); err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to revoke the refunded entitlement.")
			return
		}
	}
	if _, err := tx.AdminAuditLog.Create().SetAdminAPITokenID(adminIdentity(c).ID).SetAction("order.refund").SetResourceType("payment_order").SetResourceID(strconv.Itoa(order.ID)).SetIdempotencyKey(key).SetDiffSummaryJSON(string(details)).SetReason(strings.TrimSpace(request.Reason)).Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to write the refund audit log.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to store the refund.")
		return
	}
	statusCode := http.StatusAccepted
	if newStatus == paymentorder.StatusRefunded {
		statusCode = http.StatusOK
	}
	c.JSON(statusCode, gin.H{"id": order.ID, "status": newStatus, "refund_id": refund.ID, "refunded_at": current.RefundedAt})
}

func applySuccessfulRefund(c *gin.Context, deps Dependencies, tx *ent.Tx, order *ent.PaymentOrder, now time.Time) error {
	paths := make([]string, 0, 8)
	if order.Purpose == paymentorder.PurposeListing {
		placementRow, err := tx.Placement.Query().Where(placement.PaymentOrderIDEQ(order.ID)).Only(c.Request.Context())
		if err == nil {
			if _, err := tx.Placement.Update().Where(placement.IDEQ(placementRow.ID), placement.StatusIn(placement.StatusPending, placement.StatusActive, placement.StatusSuspended)).SetStatus(placement.StatusCanceled).SetEndsAt(now).SetSuspendedReason("payment refunded").Save(c.Request.Context()); err != nil {
				return err
			}
			siteRow, err := tx.Site.Get(c.Request.Context(), placementRow.SiteID)
			if err != nil {
				return err
			}
			locales, err := tx.PlacementLocale.Query().Where(placementlocale.PlacementIDEQ(placementRow.ID)).All(c.Request.Context())
			if err != nil {
				return err
			}
			paths = append(paths, "/")
			for _, localeRow := range locales {
				if localeRow.Locale == "zh-CN" {
					paths = append(paths, "/sites/"+siteRow.Slug)
				} else {
					paths = append(paths, "/"+localeRow.Locale, "/"+localeRow.Locale+"/sites/"+siteRow.Slug)
				}
			}
		} else if !ent.IsNotFound(err) {
			return err
		}
	} else {
		adRow, err := tx.Ad.Query().Where(ad.PaymentOrderIDEQ(order.ID)).Only(c.Request.Context())
		if err == nil {
			if _, err := tx.Ad.Update().Where(ad.IDEQ(adRow.ID), ad.StatusNEQ(ad.StatusCanceled)).SetStatus(ad.StatusCanceled).SetEndsAt(now).AddVersion(1).Save(c.Request.Context()); err != nil {
				return err
			}
			paths = append(paths, adHomepagePaths()...)
		} else if !ent.IsNotFound(err) {
			return err
		}
	}
	if len(paths) > 0 {
		if _, err := tx.RevalidationJob.Create().SetDedupeKey("order:refund:" + strconv.Itoa(order.ID) + ":" + strconv.FormatInt(now.UnixNano(), 36)).SetPaths(uniqueStrings(paths)).Save(c.Request.Context()); err != nil {
			return err
		}
	}
	if order.Purpose == paymentorder.PurposeListing && order.SubmissionID != nil {
		row, err := tx.Submission.Get(c.Request.Context(), *order.SubmissionID)
		if err != nil {
			return err
		}
		payload := notificationPayload{ApplicantName: row.ApplicantName, Email: row.ApplicantEmail, ResourceID: row.ID, ResourceType: "submission", ApplicationType: string(row.Type), Name: row.RawName, URL: row.NormalizedURL, Status: "refunded"}
		return createApplicationOutbox(c, deps, tx, "submission.refunded", row.ID, row.PreferredLocale, row.ApplicantEmail, payload)
	}
	if order.Purpose == paymentorder.PurposeAd && order.AdApplicationID != nil {
		row, err := tx.AdApplication.Get(c.Request.Context(), *order.AdApplicationID)
		if err != nil {
			return err
		}
		payload := notificationPayload{ApplicantName: row.ApplicantName, Email: row.ApplicantEmail, ResourceID: row.ID, ResourceType: "ad_application", Name: row.RawTitle, URL: row.TargetURL, Status: "refunded"}
		return createApplicationOutbox(c, deps, tx, "ad_application.refunded", row.ID, row.PreferredLocale, row.ApplicantEmail, payload)
	}
	return errors.New("refund order subject is invalid")
}

func requireIdempotencyKey(c *gin.Context) (string, bool) {
	value := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if len(value) < 8 || len(value) > 255 {
		writeError(c, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key must contain 8 to 255 characters.")
		return "", false
	}
	return value, true
}

func idempotencyDigest(value string) string { return stringDigest(value)[:32] }

func stringDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
