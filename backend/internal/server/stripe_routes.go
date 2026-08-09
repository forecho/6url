package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/adapplication"
	"github.com/forecho/6url/backend/ent/paymentevent"
	"github.com/forecho/6url/backend/ent/paymentorder"
	"github.com/forecho/6url/backend/ent/submission"
	"github.com/forecho/6url/backend/internal/stripeclient"
)

func createCheckout(c *gin.Context, deps Dependencies, subjectType string) {
	session, ok := authenticateApplicationSession(c, deps, subjectType)
	if !ok || !validateSessionWrite(c, deps, session) {
		return
	}
	query := deps.Ent.PaymentOrder.Query().Where(paymentorder.StatusIn(paymentorder.StatusPending, paymentorder.StatusProcessing))
	statusPath := "/submissions/status"
	var email string
	if subjectType == "submission" {
		query = query.Where(paymentorder.SubmissionIDEQ(*session.SubmissionID), paymentorder.PurposeEQ(paymentorder.PurposeListing))
		row, err := deps.Ent.Submission.Get(c.Request.Context(), *session.SubmissionID)
		if err != nil || row.Status != submission.StatusAwaitingPayment {
			writeError(c, http.StatusConflict, "invalid_state", "The submission is not awaiting payment.")
			return
		}
		email = row.ApplicantEmail
	} else {
		query = query.Where(paymentorder.AdApplicationIDEQ(*session.AdApplicationID), paymentorder.PurposeEQ(paymentorder.PurposeAd))
		statusPath = "/advertise/status"
		row, err := deps.Ent.AdApplication.Get(c.Request.Context(), *session.AdApplicationID)
		if err != nil || row.Status != adapplication.StatusAwaitingPayment {
			writeError(c, http.StatusConflict, "invalid_state", "The advertising application is not awaiting payment.")
			return
		}
		email = row.ApplicantEmail
	}
	order, err := query.Only(c.Request.Context())
	if err != nil || order.StripePriceIDSnapshot == nil {
		writeError(c, http.StatusConflict, "order_unavailable", "A payable Stripe order is unavailable.")
		return
	}
	checkout, err := deps.Stripe.CreateCheckout(c.Request.Context(), stripeclient.CheckoutRequest{OrderID: order.ID, PriceID: *order.StripePriceIDSnapshot, CustomerEmail: email, SuccessURL: deps.Config.AppBaseURL + statusPath + "?payment=success", CancelURL: deps.Config.AppBaseURL + statusPath + "?payment=canceled"})
	if err != nil {
		writeError(c, http.StatusBadGateway, "payment_provider_error", "Unable to create the Stripe Checkout session.")
		return
	}
	changed, err := deps.Ent.PaymentOrder.Update().Where(paymentorder.IDEQ(order.ID), paymentorder.StatusIn(paymentorder.StatusPending, paymentorder.StatusProcessing)).SetStatus(paymentorder.StatusProcessing).SetStripeCheckoutSessionID(checkout.ID).Save(c.Request.Context())
	if err != nil || changed != 1 {
		writeError(c, http.StatusConflict, "concurrent_update", "The payment order changed while Checkout was being created.")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusCreated, gin.H{"checkout_url": checkout.URL, "order_id": order.ID})
}

type stripeEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object struct {
			ID            string            `json:"id"`
			PaymentStatus string            `json:"payment_status"`
			PaymentIntent string            `json:"payment_intent"`
			AmountTotal   int64             `json:"amount_total"`
			Currency      string            `json:"currency"`
			Livemode      bool              `json:"livemode"`
			Metadata      map[string]string `json:"metadata"`
		} `json:"object"`
	} `json:"data"`
}

func receiveStripeWebhook(c *gin.Context, deps Dependencies) {
	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, 1024*1024+1))
	if err != nil || len(payload) > 1024*1024 {
		writeError(c, http.StatusBadRequest, "invalid_webhook", "The webhook payload is invalid.")
		return
	}
	if err := stripeclient.VerifyWebhook(payload, c.GetHeader("Stripe-Signature"), deps.Config.StripeWebhookSecret, time.Now()); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_signature", "The Stripe webhook signature is invalid.")
		return
	}
	var event stripeEvent
	if json.Unmarshal(payload, &event) != nil || event.ID == "" || event.Type == "" {
		writeError(c, http.StatusBadRequest, "invalid_webhook", "The Stripe webhook payload is invalid.")
		return
	}
	if exists, _ := deps.Ent.PaymentEvent.Query().Where(paymentevent.EventIDEQ(event.ID)).Exist(c.Request.Context()); exists {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}
	digest := sha256.Sum256(payload)
	if event.Type != "checkout.session.completed" || event.Data.Object.PaymentStatus != "paid" {
		if _, err := deps.Ent.PaymentEvent.Create().SetProvider("stripe").SetEventID(event.ID).SetType(event.Type).SetPayloadEncryptedOrRedacted("sha256:" + hex.EncodeToString(digest[:])).SetReceivedAt(time.Now()).SetProcessedAt(time.Now()).Save(c.Request.Context()); err != nil && !ent.IsConstraintError(err) {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to record the Stripe webhook.")
			return
		}
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}
	orderID, err := strconv.Atoi(event.Data.Object.Metadata["order_id"])
	if err != nil || orderID <= 0 {
		writeError(c, http.StatusBadRequest, "invalid_webhook", "The Stripe Checkout session has no valid order reference.")
		return
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to process the Stripe webhook.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.PaymentEvent.Create().SetProvider("stripe").SetEventID(event.ID).SetType(event.Type).SetPayloadEncryptedOrRedacted("sha256:" + hex.EncodeToString(digest[:])).SetReceivedAt(time.Now()).SetProcessingStartedAt(time.Now()).Save(c.Request.Context()); err != nil {
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusOK, gin.H{"received": true})
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to record the Stripe webhook.")
		return
	}
	order, err := tx.PaymentOrder.Get(c.Request.Context(), orderID)
	if err != nil || order.StripeCheckoutSessionID == nil || *order.StripeCheckoutSessionID != event.Data.Object.ID {
		writeError(c, http.StatusBadRequest, "order_mismatch", "The Stripe Checkout session does not match the payment order.")
		return
	}
	if order.AmountMinor != event.Data.Object.AmountTotal || order.Currency != event.Data.Object.Currency || order.Livemode != event.Data.Object.Livemode {
		writeError(c, http.StatusBadRequest, "order_mismatch", "The Stripe Checkout amount, currency, or livemode does not match the payment order.")
		return
	}
	if order.Status == paymentorder.StatusPaid {
		_, _ = tx.PaymentEvent.Update().Where(paymentevent.EventIDEQ(event.ID)).SetProcessedAt(time.Now()).Save(c.Request.Context())
		if err := tx.Commit(); err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to finish the Stripe webhook.")
			return
		}
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}
	changed, err := tx.PaymentOrder.Update().Where(paymentorder.IDEQ(order.ID), paymentorder.StatusIn(paymentorder.StatusPending, paymentorder.StatusProcessing)).SetStatus(paymentorder.StatusPaid).SetPaidAt(time.Now()).SetStripePaymentIntentID(event.Data.Object.PaymentIntent).Save(c.Request.Context())
	if err != nil || changed != 1 {
		writeError(c, http.StatusConflict, "concurrent_update", "The payment order changed while the webhook was processed.")
		return
	}
	if err := markApplicationPaid(c, deps, tx, order); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to mark the application as paid.")
		return
	}
	_, _ = tx.PaymentEvent.Update().Where(paymentevent.EventIDEQ(event.ID)).SetProcessedAt(time.Now()).Save(c.Request.Context())
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to process the Stripe webhook.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"received": true})
}

func markApplicationPaid(c *gin.Context, deps Dependencies, tx *ent.Tx, order *ent.PaymentOrder) error {
	if order.Purpose == paymentorder.PurposeListing && order.SubmissionID != nil {
		row, err := tx.Submission.Get(c.Request.Context(), *order.SubmissionID)
		if err != nil {
			return err
		}
		changed, err := tx.Submission.Update().Where(submission.IDEQ(row.ID), submission.StatusEQ(submission.StatusAwaitingPayment)).SetStatus(submission.StatusPaid).Save(c.Request.Context())
		if err != nil || changed != 1 {
			return errors.New("submission is not awaiting payment")
		}
		payload := notificationPayload{ApplicantName: row.ApplicantName, Email: row.ApplicantEmail, ResourceID: row.ID, ResourceType: "submission", ApplicationType: string(row.Type), Name: row.RawName, URL: row.NormalizedURL, Status: string(submission.StatusPaid)}
		return createApplicationOutbox(c, deps, tx, "submission.paid", row.ID, row.PreferredLocale, row.ApplicantEmail, payload)
	}
	if order.Purpose == paymentorder.PurposeAd && order.AdApplicationID != nil {
		row, err := tx.AdApplication.Get(c.Request.Context(), *order.AdApplicationID)
		if err != nil {
			return err
		}
		changed, err := tx.AdApplication.Update().Where(adapplication.IDEQ(row.ID), adapplication.StatusEQ(adapplication.StatusAwaitingPayment)).SetStatus(adapplication.StatusPaid).Save(c.Request.Context())
		if err != nil || changed != 1 {
			return errors.New("advertising application is not awaiting payment")
		}
		payload := notificationPayload{ApplicantName: row.ApplicantName, Email: row.ApplicantEmail, ResourceID: row.ID, ResourceType: "ad_application", Name: row.RawTitle, URL: row.TargetURL, Status: string(adapplication.StatusPaid)}
		return createApplicationOutbox(c, deps, tx, "ad_application.paid", row.ID, row.PreferredLocale, row.ApplicantEmail, payload)
	}
	return errors.New("payment order subject is invalid")
}
