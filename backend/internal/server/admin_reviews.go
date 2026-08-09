package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/adapplication"
	"github.com/forecho/6url/backend/ent/adoffer"
	"github.com/forecho/6url/backend/ent/listingplan"
	"github.com/forecho/6url/backend/ent/paymentorder"
	"github.com/forecho/6url/backend/ent/submission"
)

type reviewRequest struct {
	Note string `json:"note"`
}

func reviewSubmission(c *gin.Context, deps Dependencies, action string) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var request reviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "The review payload is invalid.")
		return
	}
	note := strings.TrimSpace(request.Note)
	if (action == "request_changes" || action == "reject") && !validTrimmedLength(note, 500) {
		writeError(c, http.StatusBadRequest, "review_note_required", "A review note of at most 500 characters is required.")
		return
	}
	if action == "preapprove" && len([]rune(note)) > 500 {
		writeError(c, http.StatusBadRequest, "invalid_review_note", "The review note may contain at most 500 characters.")
		return
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to review the submission.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	row, err := tx.Submission.Get(c.Request.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			writeError(c, http.StatusNotFound, "not_found", "The submission does not exist.")
		} else {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to review the submission.")
		}
		return
	}
	if row.Status != submission.StatusReceived && row.Status != submission.StatusNeedsChanges {
		writeError(c, http.StatusConflict, "invalid_state", "The submission cannot transition from its current state.")
		return
	}
	newStatus := submission.StatusRejected
	if action == "request_changes" {
		newStatus = submission.StatusNeedsChanges
	}
	var orderID *int
	if action == "preapprove" {
		if row.Type == submission.TypeExchange {
			newStatus = submission.StatusExchangeVerification
		} else {
			plan, err := tx.ListingPlan.Query().Where(listingplan.ActiveEQ(true), listingplan.CurrencyEQ("usd")).Only(c.Request.Context())
			if err != nil || plan.StripePriceID == nil {
				writeError(c, http.StatusConflict, "offer_unavailable", "Exactly one active USD listing plan with a Stripe price is required.")
				return
			}
			orderCreator := tx.PaymentOrder.Create().SetPurpose(paymentorder.PurposeListing).SetSubmissionID(row.ID).SetListingPlanID(plan.ID).SetProvider(paymentorder.ProviderStripe).SetAmountMinor(plan.AmountMinor).SetCurrency("usd").SetLocalesSnapshot(plan.IncludedLocales).SetStripePriceIDSnapshot(*plan.StripePriceID).SetLivemode(deps.Config.Environment == "production")
			if plan.DurationDays != nil {
				orderCreator.SetDurationDaysSnapshot(*plan.DurationDays)
			}
			order, err := orderCreator.Save(c.Request.Context())
			if err != nil {
				writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create the listing order.")
				return
			}
			orderID = &order.ID
			newStatus = submission.StatusAwaitingPayment
		}
	}
	changed, err := tx.Submission.Update().Where(submission.IDEQ(row.ID), submission.StatusEQ(row.Status)).SetStatus(newStatus).SetReviewNote(note).SetReviewerTokenID(adminIdentity(c).ID).SetReviewedAt(time.Now()).Save(c.Request.Context())
	if err != nil || changed != 1 {
		writeError(c, http.StatusConflict, "concurrent_update", "The submission changed during review.")
		return
	}
	if err := createReviewAudit(c, tx, action, "submission", row.ID, string(row.Status), string(newStatus), note); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to write the review audit log.")
		return
	}
	payload := notificationPayload{ApplicantName: row.ApplicantName, Email: row.ApplicantEmail, ResourceID: row.ID, ResourceType: "submission", ApplicationType: string(row.Type), Name: row.RawName, URL: row.NormalizedURL, Status: string(newStatus), ReviewNote: note}
	if deps.PayloadSealer == nil || createApplicationOutbox(c, deps, tx, "submission."+string(newStatus), row.ID, row.PreferredLocale, row.ApplicantEmail, payload) != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to queue the review notification.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to review the submission.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": row.ID, "status": newStatus, "payment_order_id": orderID})
}

func reviewAdApplication(c *gin.Context, deps Dependencies, action string) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var request reviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "The review payload is invalid.")
		return
	}
	note := strings.TrimSpace(request.Note)
	if (action == "request_changes" || action == "reject") && !validTrimmedLength(note, 500) {
		writeError(c, http.StatusBadRequest, "review_note_required", "A review note of at most 500 characters is required.")
		return
	}
	if action == "preapprove" && len([]rune(note)) > 500 {
		writeError(c, http.StatusBadRequest, "invalid_review_note", "The review note may contain at most 500 characters.")
		return
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to review the advertising application.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	row, err := tx.AdApplication.Get(c.Request.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			writeError(c, http.StatusNotFound, "not_found", "The advertising application does not exist.")
		} else {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to review the advertising application.")
		}
		return
	}
	if row.Status != adapplication.StatusReceived && row.Status != adapplication.StatusNeedsChanges {
		writeError(c, http.StatusConflict, "invalid_state", "The advertising application cannot transition from its current state.")
		return
	}
	newStatus := adapplication.StatusRejected
	if action == "request_changes" {
		newStatus = adapplication.StatusNeedsChanges
	}
	var orderID *int
	if action == "preapprove" {
		offer, err := tx.AdOffer.Query().Where(adoffer.ActiveEQ(true), adoffer.CurrencyEQ("usd")).Only(c.Request.Context())
		if err != nil || offer.StripePriceID == nil {
			writeError(c, http.StatusConflict, "offer_unavailable", "Exactly one active USD sponsor offer with a Stripe price is required.")
			return
		}
		order, err := tx.PaymentOrder.Create().SetPurpose(paymentorder.PurposeAd).SetAdApplicationID(row.ID).SetAdOfferID(offer.ID).SetProvider(paymentorder.ProviderStripe).SetAmountMinor(offer.AmountMinor).SetCurrency("usd").SetDurationDaysSnapshot(offer.DurationDays).SetLocalesSnapshot(offer.IncludedLocales).SetStripePriceIDSnapshot(*offer.StripePriceID).SetLivemode(deps.Config.Environment == "production").Save(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create the sponsor order.")
			return
		}
		orderID = &order.ID
		newStatus = adapplication.StatusAwaitingPayment
	}
	changed, err := tx.AdApplication.Update().Where(adapplication.IDEQ(row.ID), adapplication.StatusEQ(row.Status)).SetStatus(newStatus).SetReviewNote(note).SetReviewerTokenID(adminIdentity(c).ID).Save(c.Request.Context())
	if err != nil || changed != 1 {
		writeError(c, http.StatusConflict, "concurrent_update", "The advertising application changed during review.")
		return
	}
	if err := createReviewAudit(c, tx, action, "ad_application", row.ID, string(row.Status), string(newStatus), note); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to write the review audit log.")
		return
	}
	payload := notificationPayload{ApplicantName: row.ApplicantName, Email: row.ApplicantEmail, ResourceID: row.ID, ResourceType: "ad_application", Name: row.RawTitle, URL: row.TargetURL, Status: string(newStatus), ReviewNote: note}
	if deps.PayloadSealer == nil || createApplicationOutbox(c, deps, tx, "ad_application."+string(newStatus), row.ID, row.PreferredLocale, row.ApplicantEmail, payload) != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to queue the review notification.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to review the advertising application.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": row.ID, "status": newStatus, "payment_order_id": orderID})
}

func createReviewAudit(c *gin.Context, tx *ent.Tx, action, resourceType string, id int, from, to, note string) error {
	diff, _ := json.Marshal(map[string]string{"from": from, "to": to})
	_, err := tx.AdminAuditLog.Create().SetAdminAPITokenID(adminIdentity(c).ID).SetAction(resourceType + "." + action).SetResourceType(resourceType).SetResourceID(strconv.Itoa(id)).SetRequestID(c.GetHeader("X-Request-ID")).SetNillableIdempotencyKey(optionalString(c.GetHeader("Idempotency-Key"))).SetDiffSummaryJSON(string(diff)).SetReason(note).Save(c.Request.Context())
	return err
}

func resourceID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		writeError(c, http.StatusBadRequest, "invalid_id", "The resource ID is invalid.")
		return 0, false
	}
	return id, true
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
