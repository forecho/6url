package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/adapplication"
	"github.com/forecho/6url/backend/ent/applicationsession"
	"github.com/forecho/6url/backend/ent/category"
	"github.com/forecho/6url/backend/ent/notificationdelivery"
	"github.com/forecho/6url/backend/ent/site"
	"github.com/forecho/6url/backend/ent/submission"
	"github.com/forecho/6url/backend/internal/publictoken"
	"github.com/forecho/6url/backend/internal/siteurl"
)

const (
	submissionCookie            = "__Host-6url-submission"
	adCookie                    = "__Host-6url-ad"
	developmentSubmissionCookie = "6url-submission"
	developmentAdCookie         = "6url-ad"
	statusSessionTTL            = 30 * time.Minute
)

var supportedApplicationLocales = map[string]struct{}{"zh-CN": {}, "en": {}, "ja": {}, "ko": {}}

type submissionRequest struct {
	Type            string `json:"type" binding:"required"`
	ApplicantEmail  string `json:"applicant_email" binding:"required"`
	ApplicantName   string `json:"applicant_name" binding:"required"`
	PreferredLocale string `json:"preferred_locale" binding:"required"`
	Name            string `json:"name" binding:"required"`
	URL             string `json:"url" binding:"required"`
	Description     string `json:"description" binding:"required"`
	CategoryID      int    `json:"category_id" binding:"required"`
	BacklinkURL     string `json:"backlink_url"`
	TurnstileToken  string `json:"turnstile_token" binding:"required"`
}

type adApplicationRequest struct {
	ApplicantEmail   string   `json:"applicant_email" binding:"required"`
	ApplicantName    string   `json:"applicant_name" binding:"required"`
	PreferredLocale  string   `json:"preferred_locale" binding:"required"`
	TargetURL        string   `json:"target_url" binding:"required"`
	RequestedLocales []string `json:"requested_locales" binding:"required,min=1"`
	Title            string   `json:"title" binding:"required"`
	Description      string   `json:"description" binding:"required"`
	SourceImageURL   string   `json:"source_image_url"`
	TurnstileToken   string   `json:"turnstile_token" binding:"required"`
}

type sessionExchangeRequest struct {
	Token string `json:"token" binding:"required"`
}

type submissionChangesRequest struct {
	Name        string `json:"name" binding:"required"`
	URL         string `json:"url" binding:"required"`
	Description string `json:"description" binding:"required"`
	CategoryID  int    `json:"category_id" binding:"required"`
	BacklinkURL string `json:"backlink_url"`
}

type adChangesRequest struct {
	TargetURL        string   `json:"target_url" binding:"required"`
	RequestedLocales []string `json:"requested_locales" binding:"required,min=1"`
	Title            string   `json:"title" binding:"required"`
	Description      string   `json:"description" binding:"required"`
	SourceImageURL   string   `json:"source_image_url"`
}

type notificationPayload struct {
	ApplicantName   string `json:"applicant_name,omitempty"`
	Email           string `json:"email,omitempty"`
	ResourceID      int    `json:"resource_id"`
	ResourceType    string `json:"resource_type"`
	ApplicationType string `json:"application_type,omitempty"`
	Name            string `json:"name,omitempty"`
	URL             string `json:"url,omitempty"`
	Status          string `json:"status"`
	StatusURL       string `json:"status_url,omitempty"`
	ReviewNote      string `json:"review_note,omitempty"`
}

func registerPublicApplicationRoutes(public *gin.RouterGroup, deps Dependencies) {
	public.GET("/openapi.json", servePublicOpenAPI)
	public.POST("/submissions", func(c *gin.Context) { createSubmission(c, deps) })
	public.POST("/ad-applications", func(c *gin.Context) { createAdApplication(c, deps) })
	public.POST("/submission-session", func(c *gin.Context) { exchangeSubmissionSession(c, deps) })
	public.DELETE("/submission-session", func(c *gin.Context) { deleteApplicationSession(c, deps, "submission") })
	public.GET("/submissions/me", func(c *gin.Context) { getSubmissionStatus(c, deps) })
	public.PATCH("/submissions/me", func(c *gin.Context) { updateSubmission(c, deps) })
	public.POST("/ad-application-session", func(c *gin.Context) { exchangeAdSession(c, deps) })
	public.DELETE("/ad-application-session", func(c *gin.Context) { deleteApplicationSession(c, deps, "ad_application") })
	public.GET("/ad-applications/me", func(c *gin.Context) { getAdApplicationStatus(c, deps) })
	public.PATCH("/ad-applications/me", func(c *gin.Context) { updateAdApplication(c, deps) })
	public.POST("/submissions/me/checkout", func(c *gin.Context) { createCheckout(c, deps, "submission") })
	public.POST("/ad-applications/me/checkout", func(c *gin.Context) { createCheckout(c, deps, "ad_application") })
	public.POST("/webhooks/stripe", func(c *gin.Context) { receiveStripeWebhook(c, deps) })
}

func createSubmission(c *gin.Context, deps Dependencies) {
	if !allowRequest(c, deps, "submission:create", 5, time.Hour) {
		return
	}
	var request submissionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "The submission payload is invalid.")
		return
	}
	request.Type = strings.ToLower(strings.TrimSpace(request.Type))
	if request.Type != string(submission.TypePaid) && request.Type != string(submission.TypeExchange) {
		writeError(c, http.StatusBadRequest, "invalid_type", "type must be paid or exchange.")
		return
	}
	if !validApplicant(request.ApplicantEmail, request.ApplicantName, request.PreferredLocale) || !validTrimmedLength(request.Name, 180) || !validTrimmedLength(request.Description, 20_000) {
		writeError(c, http.StatusBadRequest, "invalid_request", "The applicant or site content is invalid.")
		return
	}
	if err := deps.Turnstile.Verify(c.Request.Context(), request.TurnstileToken, c.ClientIP()); err != nil {
		writeError(c, http.StatusBadRequest, "challenge_failed", "Human verification failed.")
		return
	}
	normalized, err := siteurl.Normalize(request.URL)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_url", "url must be an absolute public HTTP or HTTPS URL.")
		return
	}
	var backlinkURL *string
	if request.Type == string(submission.TypeExchange) {
		backlink, err := siteurl.Normalize(request.BacklinkURL)
		if err != nil || backlink.RegistrableDomain != normalized.RegistrableDomain {
			writeError(c, http.StatusBadRequest, "invalid_backlink_url", "backlink_url must use the submitted site's registrable domain.")
			return
		}
		backlinkURL = &backlink.URL
	}
	validCategory, err := deps.Ent.Category.Query().Where(category.IDEQ(request.CategoryID), category.StatusEQ(category.StatusPublished)).Exist(c.Request.Context())
	if err != nil || !validCategory {
		writeError(c, http.StatusBadRequest, "invalid_category", "The selected category is unavailable.")
		return
	}
	pending, err := deps.Ent.Submission.Query().Where(submission.NormalizedRegistrableDomainEQ(normalized.RegistrableDomain), submission.StatusIn(submission.StatusReceived, submission.StatusNeedsChanges, submission.StatusPreapproved, submission.StatusExchangeVerification, submission.StatusAwaitingPayment)).Exist(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to receive the submission.")
		return
	}
	if pending {
		c.JSON(http.StatusAccepted, gin.H{"status": "received"})
		return
	}
	rawToken, err := publictoken.Generate(publictoken.Submission)
	if err != nil || deps.PayloadSealer == nil {
		writeError(c, http.StatusServiceUnavailable, "service_unavailable", "Submission notifications are unavailable.")
		return
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to receive the submission.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	creator := tx.Submission.Create().SetPublicTokenHash(publictoken.Hash(rawToken, deps.Config.ApplicationTokenPepper, publictoken.Submission)).SetType(submission.Type(request.Type)).SetApplicantEmail(strings.ToLower(strings.TrimSpace(request.ApplicantEmail))).SetApplicantName(strings.TrimSpace(request.ApplicantName)).SetPreferredLocale(request.PreferredLocale).SetRawName(strings.TrimSpace(request.Name)).SetRawURL(strings.TrimSpace(request.URL)).SetNormalizedURL(normalized.URL).SetNormalizedRegistrableDomain(normalized.RegistrableDomain).SetRawDescription(strings.TrimSpace(request.Description)).SetRequestedCategoryID(request.CategoryID).SetNillableBacklinkURL(backlinkURL)
	if duplicate, queryErr := tx.Site.Query().Where(site.CanonicalURLHashEQ(normalized.Hash)).Only(c.Request.Context()); queryErr == nil {
		creator.SetDuplicateSiteID(duplicate.ID)
	} else if !ent.IsNotFound(queryErr) {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to receive the submission.")
		return
	}
	row, err := creator.Save(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to receive the submission.")
		return
	}
	statusURL := deps.Config.AppBaseURL + "/submissions/status#token=" + rawToken
	payload := notificationPayload{ApplicantName: row.ApplicantName, Email: row.ApplicantEmail, ResourceID: row.ID, ResourceType: "submission", ApplicationType: request.Type, Name: row.RawName, URL: row.NormalizedURL, Status: string(row.Status), StatusURL: statusURL}
	if err := createApplicationOutbox(c, deps, tx, "submission.received", row.ID, row.PreferredLocale, row.ApplicantEmail, payload); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to receive the submission.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to receive the submission.")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "received"})
}

func createAdApplication(c *gin.Context, deps Dependencies) {
	if !allowRequest(c, deps, "ad_application:create", 5, time.Hour) {
		return
	}
	var request adApplicationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "The advertising application payload is invalid.")
		return
	}
	if !validApplicant(request.ApplicantEmail, request.ApplicantName, request.PreferredLocale) || !validTrimmedLength(request.Title, 180) || !validTrimmedLength(request.Description, 500) {
		writeError(c, http.StatusBadRequest, "invalid_request", "The applicant or advertisement content is invalid.")
		return
	}
	locales, ok := normalizeLocales(request.RequestedLocales)
	if !ok {
		writeError(c, http.StatusBadRequest, "unsupported_locale", "requested_locales contains an unsupported or duplicate locale.")
		return
	}
	if err := deps.Turnstile.Verify(c.Request.Context(), request.TurnstileToken, c.ClientIP()); err != nil {
		writeError(c, http.StatusBadRequest, "challenge_failed", "Human verification failed.")
		return
	}
	target, err := siteurl.Normalize(request.TargetURL)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_url", "target_url must be an absolute public HTTP or HTTPS URL.")
		return
	}
	var sourceImageURL *string
	if strings.TrimSpace(request.SourceImageURL) != "" {
		image, err := siteurl.Normalize(request.SourceImageURL)
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_image_url", "source_image_url must be an absolute public HTTP or HTTPS URL.")
			return
		}
		sourceImageURL = &image.URL
	}
	rawToken, err := publictoken.Generate(publictoken.AdApplication)
	if err != nil || deps.PayloadSealer == nil {
		writeError(c, http.StatusServiceUnavailable, "service_unavailable", "Advertising application notifications are unavailable.")
		return
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to receive the advertising application.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	row, err := tx.AdApplication.Create().SetPublicTokenHash(publictoken.Hash(rawToken, deps.Config.ApplicationTokenPepper, publictoken.AdApplication)).SetApplicantEmail(strings.ToLower(strings.TrimSpace(request.ApplicantEmail))).SetApplicantName(strings.TrimSpace(request.ApplicantName)).SetPreferredLocale(request.PreferredLocale).SetTargetURL(target.URL).SetRequestedLocales(locales).SetRawTitle(strings.TrimSpace(request.Title)).SetRawDescription(strings.TrimSpace(request.Description)).SetNillableSourceImageURL(sourceImageURL).Save(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to receive the advertising application.")
		return
	}
	statusURL := deps.Config.AppBaseURL + "/advertise/status#token=" + rawToken
	payload := notificationPayload{ApplicantName: row.ApplicantName, Email: row.ApplicantEmail, ResourceID: row.ID, ResourceType: "ad_application", Name: row.RawTitle, URL: row.TargetURL, Status: string(row.Status), StatusURL: statusURL}
	if err := createApplicationOutbox(c, deps, tx, "ad_application.received", row.ID, row.PreferredLocale, row.ApplicantEmail, payload); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to receive the advertising application.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to receive the advertising application.")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "received"})
}

func exchangeSubmissionSession(c *gin.Context, deps Dependencies) {
	var request sessionExchangeRequest
	if err := c.ShouldBindJSON(&request); err != nil || !publictoken.ValidShape(request.Token, publictoken.Submission) || !allowRequest(c, deps, "submission:session", 10, time.Hour) {
		if !c.IsAborted() {
			writeError(c, http.StatusUnauthorized, "invalid_token", "The submission status token is invalid.")
		}
		return
	}
	row, err := deps.Ent.Submission.Query().Where(submission.PublicTokenHashEQ(publictoken.Hash(request.Token, deps.Config.ApplicationTokenPepper, publictoken.Submission))).Only(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusUnauthorized, "invalid_token", "The submission status token is invalid.")
		return
	}
	csrf, err := createSession(c, deps, "submission", row.ID, 0)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create the status session.")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"csrf_token": csrf, "expires_in": int(statusSessionTTL.Seconds())})
}

func exchangeAdSession(c *gin.Context, deps Dependencies) {
	var request sessionExchangeRequest
	if err := c.ShouldBindJSON(&request); err != nil || !publictoken.ValidShape(request.Token, publictoken.AdApplication) || !allowRequest(c, deps, "ad_application:session", 10, time.Hour) {
		if !c.IsAborted() {
			writeError(c, http.StatusUnauthorized, "invalid_token", "The advertising application status token is invalid.")
		}
		return
	}
	row, err := deps.Ent.AdApplication.Query().Where(adapplication.PublicTokenHashEQ(publictoken.Hash(request.Token, deps.Config.ApplicationTokenPepper, publictoken.AdApplication))).Only(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusUnauthorized, "invalid_token", "The advertising application status token is invalid.")
		return
	}
	csrf, err := createSession(c, deps, "ad_application", 0, row.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create the status session.")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"csrf_token": csrf, "expires_in": int(statusSessionTTL.Seconds())})
}

func createSession(c *gin.Context, deps Dependencies, subjectType string, submissionID, adApplicationID int) (string, error) {
	audience := publictoken.SubmissionSession
	cookieName := applicationCookieName(deps, "submission")
	if subjectType == "ad_application" {
		audience = publictoken.AdSession
		cookieName = applicationCookieName(deps, "ad_application")
	}
	sessionToken, err := publictoken.Generate(audience)
	if err != nil {
		return "", err
	}
	csrf, err := publictoken.Generate(publictoken.CSRF)
	if err != nil {
		return "", err
	}
	creator := deps.Ent.ApplicationSession.Create().SetSubjectType(applicationsession.SubjectType(subjectType)).SetSessionHash(publictoken.Hash(sessionToken, deps.Config.ApplicationTokenPepper, audience)).SetCsrfSecretHash(publictoken.Hash(csrf, deps.Config.ApplicationTokenPepper, publictoken.CSRF)).SetExpiresAt(time.Now().Add(statusSessionTTL))
	if submissionID > 0 {
		creator.SetSubmissionID(submissionID)
	}
	if adApplicationID > 0 {
		creator.SetAdApplicationID(adApplicationID)
	}
	if _, err := creator.Save(c.Request.Context()); err != nil {
		return "", err
	}
	secure := deps.Config.Environment == "production"
	http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: sessionToken, Path: "/", MaxAge: int(statusSessionTTL.Seconds()), HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
	return csrf, nil
}

func getSubmissionStatus(c *gin.Context, deps Dependencies) {
	session, ok := authenticateApplicationSession(c, deps, "submission")
	if !ok {
		return
	}
	row, err := deps.Ent.Submission.Get(c.Request.Context(), *session.SubmissionID)
	if err != nil {
		writeError(c, http.StatusUnauthorized, "unauthorized", "A valid submission status session is required.")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"id": row.ID, "type": row.Type, "status": row.Status, "name": row.RawName, "url": row.NormalizedURL, "preferred_locale": row.PreferredLocale, "review_note": row.ReviewNote, "submitted_at": row.SubmittedAt, "reviewed_at": row.ReviewedAt, "updated_at": row.UpdatedAt})
}

func getAdApplicationStatus(c *gin.Context, deps Dependencies) {
	session, ok := authenticateApplicationSession(c, deps, "ad_application")
	if !ok {
		return
	}
	row, err := deps.Ent.AdApplication.Get(c.Request.Context(), *session.AdApplicationID)
	if err != nil {
		writeError(c, http.StatusUnauthorized, "unauthorized", "A valid advertising application status session is required.")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"id": row.ID, "status": row.Status, "title": row.RawTitle, "target_url": row.TargetURL, "requested_locales": row.RequestedLocales, "preferred_locale": row.PreferredLocale, "review_note": row.ReviewNote, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt})
}

func updateSubmission(c *gin.Context, deps Dependencies) {
	session, ok := authenticateApplicationSession(c, deps, "submission")
	if !ok || !validateSessionWrite(c, deps, session) {
		return
	}
	var request submissionChangesRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validTrimmedLength(request.Name, 180) || !validTrimmedLength(request.Description, 20_000) {
		writeError(c, http.StatusBadRequest, "invalid_request", "The submission changes are invalid.")
		return
	}
	normalized, err := siteurl.Normalize(request.URL)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_url", "url must be an absolute public HTTP or HTTPS URL.")
		return
	}
	validCategory, err := deps.Ent.Category.Query().Where(category.IDEQ(request.CategoryID), category.StatusEQ(category.StatusPublished)).Exist(c.Request.Context())
	if err != nil || !validCategory {
		writeError(c, http.StatusBadRequest, "invalid_category", "The selected category is unavailable.")
		return
	}
	row, err := deps.Ent.Submission.Get(c.Request.Context(), *session.SubmissionID)
	if err != nil || row.Status != submission.StatusNeedsChanges {
		writeError(c, http.StatusConflict, "invalid_state", "The submission is not awaiting changes.")
		return
	}
	var backlinkURL *string
	if row.Type == submission.TypeExchange {
		backlink, err := siteurl.Normalize(request.BacklinkURL)
		if err != nil || backlink.RegistrableDomain != normalized.RegistrableDomain {
			writeError(c, http.StatusBadRequest, "invalid_backlink_url", "backlink_url must use the submitted site's registrable domain.")
			return
		}
		backlinkURL = &backlink.URL
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to update the submission.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	changed, err := tx.Submission.Update().Where(submission.IDEQ(row.ID), submission.StatusEQ(submission.StatusNeedsChanges)).SetStatus(submission.StatusReceived).SetRawName(strings.TrimSpace(request.Name)).SetRawURL(strings.TrimSpace(request.URL)).SetNormalizedURL(normalized.URL).SetNormalizedRegistrableDomain(normalized.RegistrableDomain).SetRawDescription(strings.TrimSpace(request.Description)).SetRequestedCategoryID(request.CategoryID).SetNillableBacklinkURL(backlinkURL).SetReviewNote("").ClearReviewedAt().Save(c.Request.Context())
	if err != nil || changed != 1 {
		writeError(c, http.StatusConflict, "concurrent_update", "The submission changed while it was being updated.")
		return
	}
	payload := notificationPayload{ApplicantName: row.ApplicantName, Email: row.ApplicantEmail, ResourceID: row.ID, ResourceType: "submission", ApplicationType: string(row.Type), Name: strings.TrimSpace(request.Name), URL: normalized.URL, Status: string(submission.StatusReceived)}
	if deps.PayloadSealer == nil || createApplicationOutbox(c, deps, tx, "submission.resubmitted", row.ID, row.PreferredLocale, row.ApplicantEmail, payload) != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to queue the update notification.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to update the submission.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": row.ID, "status": submission.StatusReceived})
}

func updateAdApplication(c *gin.Context, deps Dependencies) {
	session, ok := authenticateApplicationSession(c, deps, "ad_application")
	if !ok || !validateSessionWrite(c, deps, session) {
		return
	}
	var request adChangesRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validTrimmedLength(request.Title, 180) || !validTrimmedLength(request.Description, 500) {
		writeError(c, http.StatusBadRequest, "invalid_request", "The advertising application changes are invalid.")
		return
	}
	locales, ok := normalizeLocales(request.RequestedLocales)
	if !ok {
		writeError(c, http.StatusBadRequest, "unsupported_locale", "requested_locales contains an unsupported or duplicate locale.")
		return
	}
	target, err := siteurl.Normalize(request.TargetURL)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_url", "target_url must be an absolute public HTTP or HTTPS URL.")
		return
	}
	var sourceImageURL *string
	if strings.TrimSpace(request.SourceImageURL) != "" {
		image, err := siteurl.Normalize(request.SourceImageURL)
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_image_url", "source_image_url must be an absolute public HTTP or HTTPS URL.")
			return
		}
		sourceImageURL = &image.URL
	}
	row, err := deps.Ent.AdApplication.Get(c.Request.Context(), *session.AdApplicationID)
	if err != nil || row.Status != adapplication.StatusNeedsChanges {
		writeError(c, http.StatusConflict, "invalid_state", "The advertising application is not awaiting changes.")
		return
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to update the advertising application.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	changed, err := tx.AdApplication.Update().Where(adapplication.IDEQ(row.ID), adapplication.StatusEQ(adapplication.StatusNeedsChanges)).SetStatus(adapplication.StatusReceived).SetTargetURL(target.URL).SetRequestedLocales(locales).SetRawTitle(strings.TrimSpace(request.Title)).SetRawDescription(strings.TrimSpace(request.Description)).SetNillableSourceImageURL(sourceImageURL).SetReviewNote("").Save(c.Request.Context())
	if err != nil || changed != 1 {
		writeError(c, http.StatusConflict, "concurrent_update", "The advertising application changed while it was being updated.")
		return
	}
	payload := notificationPayload{ApplicantName: row.ApplicantName, Email: row.ApplicantEmail, ResourceID: row.ID, ResourceType: "ad_application", Name: strings.TrimSpace(request.Title), URL: target.URL, Status: string(adapplication.StatusReceived)}
	if deps.PayloadSealer == nil || createApplicationOutbox(c, deps, tx, "ad_application.resubmitted", row.ID, row.PreferredLocale, row.ApplicantEmail, payload) != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to queue the update notification.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to update the advertising application.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": row.ID, "status": adapplication.StatusReceived})
}

func deleteApplicationSession(c *gin.Context, deps Dependencies, subjectType string) {
	session, ok := authenticateApplicationSession(c, deps, subjectType)
	if !ok {
		return
	}
	if !validOrigin(c, deps) || !publictoken.Equal(session.CsrfSecretHash, publictoken.Hash(c.GetHeader("X-CSRF-Token"), deps.Config.ApplicationTokenPepper, publictoken.CSRF)) {
		writeError(c, http.StatusForbidden, "csrf_failed", "CSRF validation failed.")
		return
	}
	if _, err := session.Update().SetRevokedAt(time.Now()).Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to end the status session.")
		return
	}
	cookieName := applicationCookieName(deps, subjectType)
	http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: deps.Config.Environment == "production", SameSite: http.SameSiteLaxMode})
	c.Status(http.StatusNoContent)
}

func validateSessionWrite(c *gin.Context, deps Dependencies, session *ent.ApplicationSession) bool {
	if !validOrigin(c, deps) || !publictoken.Equal(session.CsrfSecretHash, publictoken.Hash(c.GetHeader("X-CSRF-Token"), deps.Config.ApplicationTokenPepper, publictoken.CSRF)) {
		writeError(c, http.StatusForbidden, "csrf_failed", "CSRF validation failed.")
		return false
	}
	return true
}

func authenticateApplicationSession(c *gin.Context, deps Dependencies, subjectType string) (*ent.ApplicationSession, bool) {
	cookieName := applicationCookieName(deps, "submission")
	audience := publictoken.SubmissionSession
	if subjectType == "ad_application" {
		cookieName = applicationCookieName(deps, "ad_application")
		audience = publictoken.AdSession
	}
	cookie, err := c.Request.Cookie(cookieName)
	if err != nil || !publictoken.ValidShape(cookie.Value, audience) {
		writeError(c, http.StatusUnauthorized, "unauthorized", "A valid application status session is required.")
		return nil, false
	}
	sessionRow, err := deps.Ent.ApplicationSession.Query().Where(applicationsession.SubjectTypeEQ(applicationsession.SubjectType(subjectType)), applicationsession.SessionHashEQ(publictoken.Hash(cookie.Value, deps.Config.ApplicationTokenPepper, audience)), applicationsession.ExpiresAtGT(time.Now()), applicationsession.RevokedAtIsNil()).Only(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusUnauthorized, "unauthorized", "A valid application status session is required.")
		return nil, false
	}
	_, _ = sessionRow.Update().SetLastSeenAt(time.Now()).Save(c.Request.Context())
	return sessionRow, true
}

func createApplicationOutbox(c *gin.Context, deps Dependencies, tx *ent.Tx, kind string, resourceID int, localeCode, email string, payload notificationPayload) error {
	baseKey := strings.ReplaceAll(kind, ".", ":") + ":" + strconv.Itoa(resourceID) + ":" + strconv.FormatInt(time.Now().UnixNano(), 36)
	emailKey := baseKey + ":email"
	encrypted, err := deps.PayloadSealer.Seal(payload, emailKey)
	if err != nil {
		return err
	}
	if _, err := tx.NotificationDelivery.Create().SetDedupeKey(emailKey).SetChannel(notificationdelivery.ChannelEmail).SetKind(kind).SetRecipient(email).SetLocale(localeCode).SetTemplateKey(kind).SetPayloadEncrypted(encrypted).SetPayloadExpiresAt(time.Now().Add(7 * 24 * time.Hour)).Save(c.Request.Context()); err != nil {
		return err
	}
	if deps.Config.SlackChannelID != "" {
		slackKey := baseKey + ":slack"
		slackPayload := payload
		slackPayload.StatusURL = ""
		encrypted, err := deps.PayloadSealer.Seal(slackPayload, slackKey)
		if err != nil {
			return err
		}
		if _, err := tx.NotificationDelivery.Create().SetDedupeKey(slackKey).SetChannel(notificationdelivery.ChannelSlack).SetKind(kind).SetRecipient(deps.Config.SlackChannelID).SetLocale("en").SetTemplateKey(kind).SetPayloadEncrypted(encrypted).SetPayloadExpiresAt(time.Now().Add(7 * 24 * time.Hour)).Save(c.Request.Context()); err != nil {
			return err
		}
	}
	return nil
}

func allowRequest(c *gin.Context, deps Dependencies, action string, limit int, window time.Duration) bool {
	digest := sha256.Sum256([]byte(deps.Config.ApplicationTokenPepper + "\x00" + c.ClientIP()))
	allowed, err := deps.RateLimiter.Allow(c.Request.Context(), action+":"+hex.EncodeToString(digest[:16]), limit, window)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "rate_limiter_unavailable", "Request protection is temporarily unavailable.")
		c.Abort()
		return false
	}
	if !allowed {
		writeError(c, http.StatusTooManyRequests, "rate_limited", "Too many requests. Please try again later.")
		c.Abort()
		return false
	}
	return true
}

func validApplicant(email, name, localeCode string) bool {
	address, err := mail.ParseAddress(strings.TrimSpace(email))
	_, localeOK := supportedApplicationLocales[localeCode]
	return err == nil && strings.EqualFold(address.Address, strings.TrimSpace(email)) && len(address.Address) <= 320 && validTrimmedLength(name, 180) && localeOK
}

func normalizeLocales(values []string) ([]string, bool) {
	if len(values) == 0 || len(values) > len(supportedApplicationLocales) {
		return nil, false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, supported := supportedApplicationLocales[value]; !supported {
			return nil, false
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, false
		}
		seen[value] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, true
}

func validOrigin(c *gin.Context, deps Dependencies) bool {
	origin := strings.TrimSuffix(strings.TrimSpace(c.GetHeader("Origin")), "/")
	return origin != "" && origin == strings.TrimSuffix(deps.Config.AppBaseURL, "/")
}

func applicationCookieName(deps Dependencies, subjectType string) string {
	if deps.Config.Environment == "production" {
		if subjectType == "ad_application" {
			return adCookie
		}
		return submissionCookie
	}
	if subjectType == "ad_application" {
		return developmentAdCookie
	}
	return developmentSubmissionCookie
}

func statusETag(resourceType string, id int, updatedAt time.Time) string {
	return fmt.Sprintf(`W/"%s-%d-%d"`, resourceType, id, updatedAt.UnixNano())
}
