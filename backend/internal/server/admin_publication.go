package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/asset"
	"github.com/forecho/6url/backend/ent/backlinkcheck"
	"github.com/forecho/6url/backend/ent/category"
	"github.com/forecho/6url/backend/ent/paymentorder"
	"github.com/forecho/6url/backend/ent/placement"
	"github.com/forecho/6url/backend/ent/site"
	"github.com/forecho/6url/backend/ent/sitecategory"
	"github.com/forecho/6url/backend/ent/sitecontentrevision"
	"github.com/forecho/6url/backend/ent/sitelocalepublication"
	"github.com/forecho/6url/backend/ent/submission"
	"github.com/forecho/6url/backend/internal/backlink"
	"github.com/forecho/6url/backend/internal/siteurl"
)

var (
	errPublicationSlugConflict = errors.New("publication slug conflicts with the existing site")
	errPlacementExists         = errors.New("a placement already exists for this submission")
)

type publishSubmissionRequest struct {
	Slug          string                   `json:"slug" binding:"required"`
	LogoAssetID   *int                     `json:"logo_asset_id"`
	Localizations []manualSiteLocalization `json:"localizations" binding:"required,len=4"`
}

func publishSubmission(c *gin.Context, deps Dependencies) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var request publishSubmissionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "The publication payload is invalid.")
		return
	}
	request.Slug = strings.TrimSpace(request.Slug)
	if len(request.Slug) > 180 || !validSlug.MatchString(request.Slug) {
		writeError(c, http.StatusBadRequest, "invalid_slug", "slug must use lowercase ASCII letters, numbers, and single hyphens.")
		return
	}
	if !validateCompleteSiteLocalizations(request.Localizations) {
		writeError(c, http.StatusBadRequest, "invalid_localizations", "Publication requires one valid localization for each supported locale.")
		return
	}
	if request.LogoAssetID != nil {
		valid, err := deps.Ent.Asset.Query().Where(asset.IDEQ(*request.LogoAssetID), asset.ProviderEQ("imgant"), asset.AlbumEQ("6url"), asset.StatusEQ(asset.StatusHealthy)).Exist(c.Request.Context())
		if err != nil || !valid {
			writeError(c, http.StatusBadRequest, "invalid_asset", "The logo must be a healthy ImgAnt asset in the 6url album.")
			return
		}
	}
	application, err := deps.Ent.Submission.Get(c.Request.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			writeError(c, http.StatusNotFound, "not_found", "The submission does not exist.")
		} else {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the submission.")
		}
		return
	}
	if application.RequestedCategoryID == nil {
		writeError(c, http.StatusConflict, "invalid_submission", "The submission has no publication category.")
		return
	}
	validCategory, err := deps.Ent.Category.Query().Where(category.IDEQ(*application.RequestedCategoryID), category.StatusEQ(category.StatusPublished)).Exist(c.Request.Context())
	if err != nil || !validCategory {
		writeError(c, http.StatusConflict, "invalid_category", "The publication category is unavailable.")
		return
	}
	var initialCheck *backlink.Result
	if application.Type == submission.TypeExchange {
		if application.Status != submission.StatusExchangeVerification || application.BacklinkURL == nil {
			writeError(c, http.StatusConflict, "invalid_state", "The exchange submission is not ready for backlink verification.")
			return
		}
		result := deps.Backlinks.Check(c.Request.Context(), *application.BacklinkURL, deps.Config.AppBaseURL)
		initialCheck = &result
		if result.Status != "passed" {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"code": "backlink_verification_failed", "status": result.Status, "error_code": result.ErrorCode, "checked_at": result.CheckedAt})
			return
		}
	} else if application.Status != submission.StatusPaid {
		writeError(c, http.StatusConflict, "invalid_state", "The paid submission is not ready for publication.")
		return
	}
	normalized, err := siteurl.Normalize(application.NormalizedURL)
	if err != nil {
		writeError(c, http.StatusConflict, "invalid_submission", "The stored submission URL is invalid.")
		return
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to publish the submission.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	siteRow, err := findOrCreatePublicationSite(c, tx, application, normalized, request)
	if err != nil {
		if ent.IsConstraintError(err) || errors.Is(err, errPublicationSlugConflict) {
			writeError(c, http.StatusConflict, "site_conflict", "The site URL or slug conflicts with an existing site.")
		} else {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create or update the published site.")
		}
		return
	}
	if err := publishSiteLocalizations(c, tx, siteRow.ID, request.Localizations); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to publish localized site content.")
		return
	}
	if err := ensureSiteCategory(c, tx, siteRow.ID, *application.RequestedCategoryID); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to assign the publication category.")
		return
	}
	placementRow, orderID, err := createSubmissionPlacement(c, tx, application, siteRow.ID, request.Localizations)
	if err != nil {
		writeError(c, http.StatusConflict, "placement_failed", "Unable to create the publication placement.")
		return
	}
	if initialCheck != nil {
		if err := saveBacklinkCheck(c, tx, placementRow.ID, *initialCheck); err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to store the backlink verification.")
			return
		}
	}
	changed, err := tx.Submission.Update().Where(submission.IDEQ(application.ID), submission.StatusEQ(application.Status)).SetStatus(submission.StatusPublished).Save(c.Request.Context())
	if err != nil || changed != 1 {
		writeError(c, http.StatusConflict, "concurrent_update", "The submission changed while it was being published.")
		return
	}
	if _, err := tx.AdminAuditLog.Create().SetAdminAPITokenID(adminIdentity(c).ID).SetAction("submission.publish").SetResourceType("submission").SetResourceID(strconv.Itoa(application.ID)).SetReason("published site and commercial placement").Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to write the publication audit log.")
		return
	}
	paths := publicationPaths(siteRow.Slug, request.Localizations)
	if _, err := tx.RevalidationJob.Create().SetDedupeKey("submission:publish:" + strconv.Itoa(application.ID) + ":" + strconv.FormatInt(time.Now().UnixNano(), 36)).SetPaths(paths).Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to queue frontend revalidation.")
		return
	}
	payload := notificationPayload{ApplicantName: application.ApplicantName, Email: application.ApplicantEmail, ResourceID: application.ID, ResourceType: "submission", ApplicationType: string(application.Type), Name: application.RawName, URL: application.NormalizedURL, Status: string(submission.StatusPublished)}
	if err := createApplicationOutbox(c, deps, tx, "submission.published", application.ID, application.PreferredLocale, application.ApplicantEmail, payload); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to queue the publication notification.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to publish the submission.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"submission_id": application.ID, "site_id": siteRow.ID, "placement_id": placementRow.ID, "payment_order_id": orderID, "status": submission.StatusPublished, "paths": paths})
}

func findOrCreatePublicationSite(c *gin.Context, tx *ent.Tx, application *ent.Submission, normalized siteurl.Normalized, request publishSubmissionRequest) (*ent.Site, error) {
	row, err := tx.Site.Query().Where(site.CanonicalURLHashEQ(normalized.Hash)).Only(c.Request.Context())
	if err == nil {
		if row.Slug != request.Slug {
			return nil, errPublicationSlugConflict
		}
		updater := row.Update().SetStatus(site.StatusPublished).SetCanonicalURL(normalized.URL).SetRegistrableDomain(normalized.RegistrableDomain).AddVersion(1)
		if request.LogoAssetID != nil {
			updater.SetLogoAssetID(*request.LogoAssetID)
		}
		return updater.Save(c.Request.Context())
	}
	if !ent.IsNotFound(err) {
		return nil, err
	}
	creator := tx.Site.Create().SetCanonicalURL(normalized.URL).SetCanonicalURLHash(normalized.Hash).SetRegistrableDomain(normalized.RegistrableDomain).SetSlug(request.Slug).SetStatus(site.StatusPublished).SetCreatedVia(site.CreatedViaSubmission)
	if request.LogoAssetID != nil {
		creator.SetLogoAssetID(*request.LogoAssetID)
	}
	return creator.Save(c.Request.Context())
}

func publishSiteLocalizations(c *gin.Context, tx *ent.Tx, siteID int, localizations []manualSiteLocalization) error {
	for _, localization := range localizations {
		latest, err := tx.SiteContentRevision.Query().Where(sitecontentrevision.SiteIDEQ(siteID), sitecontentrevision.LocaleEQ(localization.Locale)).Order(ent.Desc(sitecontentrevision.FieldRevision)).First(c.Request.Context())
		revisionNumber := 1
		if err == nil {
			revisionNumber = latest.Revision + 1
		} else if !ent.IsNotFound(err) {
			return err
		}
		revision, err := tx.SiteContentRevision.Create().SetSiteID(siteID).SetLocale(localization.Locale).SetRevision(revisionNumber).SetStatus(sitecontentrevision.StatusApproved).SetName(strings.TrimSpace(localization.Name)).SetTagline(strings.TrimSpace(localization.Tagline)).SetSummary(strings.TrimSpace(localization.Summary)).SetDescription(strings.TrimSpace(localization.Description)).SetSeoTitle(strings.TrimSpace(localization.SeoTitle)).SetSeoDescription(strings.TrimSpace(localization.SeoDescription)).SetSourceLocale(localization.Locale).SetCreatedByTokenID(adminIdentity(c).ID).Save(c.Request.Context())
		if err != nil {
			return err
		}
		publication, err := tx.SiteLocalePublication.Query().Where(sitelocalepublication.SiteIDEQ(siteID), sitelocalepublication.LocaleEQ(localization.Locale)).Only(c.Request.Context())
		if ent.IsNotFound(err) {
			_, err = tx.SiteLocalePublication.Create().SetSiteID(siteID).SetLocale(localization.Locale).SetDraftRevisionID(revision.ID).SetPublishedRevisionID(revision.ID).SetTranslationStatus(sitelocalepublication.TranslationStatusPublished).Save(c.Request.Context())
		} else if err == nil {
			_, err = publication.Update().SetDraftRevisionID(revision.ID).SetPublishedRevisionID(revision.ID).SetTranslationStatus(sitelocalepublication.TranslationStatusPublished).AddVersion(1).Save(c.Request.Context())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func ensureSiteCategory(c *gin.Context, tx *ent.Tx, siteID, categoryID int) error {
	exists, err := tx.SiteCategory.Query().Where(sitecategory.SiteIDEQ(siteID), sitecategory.CategoryIDEQ(categoryID)).Exist(c.Request.Context())
	if err != nil || exists {
		return err
	}
	count, err := tx.SiteCategory.Query().Where(sitecategory.SiteIDEQ(siteID)).Count(c.Request.Context())
	if err != nil {
		return err
	}
	_, err = tx.SiteCategory.Create().SetSiteID(siteID).SetCategoryID(categoryID).SetIsPrimary(count == 0).Save(c.Request.Context())
	return err
}

func createSubmissionPlacement(c *gin.Context, tx *ent.Tx, application *ent.Submission, siteID int, localizations []manualSiteLocalization) (*ent.Placement, *int, error) {
	exists, err := tx.Placement.Query().Where(placement.SubmissionIDEQ(application.ID)).Exist(c.Request.Context())
	if err != nil || exists {
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errPlacementExists
	}
	now := time.Now().UTC()
	creator := tx.Placement.Create().SetSiteID(siteID).SetOrigin(placement.OriginSubmission).SetSubmissionID(application.ID).SetStatus(placement.StatusActive).SetOutboundURL(application.NormalizedURL).SetStartsAt(now)
	var orderID *int
	if application.Type == submission.TypePaid {
		order, err := tx.PaymentOrder.Query().Where(paymentorder.SubmissionIDEQ(application.ID), paymentorder.PurposeEQ(paymentorder.PurposeListing), paymentorder.StatusEQ(paymentorder.StatusPaid)).Only(c.Request.Context())
		if err != nil {
			return nil, nil, err
		}
		creator.SetKind(placement.KindPaid).SetPaymentOrderID(order.ID).SetOutboundRel("sponsored nofollow noopener").SetDisclosureKey("paid")
		if order.DurationDaysSnapshot != nil {
			creator.SetEndsAt(now.AddDate(0, 0, *order.DurationDaysSnapshot))
		}
		orderID = &order.ID
		if _, err := order.Update().SetSiteID(siteID).Save(c.Request.Context()); err != nil {
			return nil, nil, err
		}
	} else {
		creator.SetKind(placement.KindExchange).SetOutboundRel("nofollow noopener").SetDisclosureKey("exchange").SetNillableBacklinkURL(application.BacklinkURL).SetLastCheckStatus("passed").SetLastCheckedAt(now)
	}
	row, err := creator.Save(c.Request.Context())
	if err != nil {
		return nil, nil, err
	}
	for _, localization := range localizations {
		if _, err := tx.PlacementLocale.Create().SetPlacementID(row.ID).SetLocale(localization.Locale).Save(c.Request.Context()); err != nil {
			return nil, nil, err
		}
	}
	return row, orderID, nil
}

func saveBacklinkCheck(c *gin.Context, tx *ent.Tx, placementID int, result backlink.Result) error {
	creator := tx.BacklinkCheck.Create().SetPlacementID(placementID).SetStatus(backlinkStatus(result.Status)).SetRequestedURL(result.RequestedURL).SetFinalURL(result.FinalURL).SetMatchedHref(result.MatchedHref).SetMatchedAnchor(result.MatchedAnchor).SetMatchedRel(result.MatchedRel).SetRedirectChainJSON(result.RedirectChainJSON()).SetErrorCode(result.ErrorCode).SetCheckedAt(result.CheckedAt)
	if result.HTTPStatus != nil {
		creator.SetHTTPStatus(*result.HTTPStatus)
	}
	_, err := creator.Save(c.Request.Context())
	return err
}

func backlinkStatus(value string) backlinkcheck.Status {
	switch value {
	case "passed":
		return backlinkcheck.StatusPassed
	case "missing":
		return backlinkcheck.StatusMissing
	case "blocked":
		return backlinkcheck.StatusBlocked
	case "unsafe":
		return backlinkcheck.StatusUnsafe
	default:
		return backlinkcheck.StatusNetworkError
	}
}

func validateCompleteSiteLocalizations(values []manualSiteLocalization) bool {
	if len(values) != len(supportedApplicationLocales) {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, supported := supportedApplicationLocales[value.Locale]; !supported || !validManualLocalization(value) {
			return false
		}
		if _, duplicate := seen[value.Locale]; duplicate {
			return false
		}
		seen[value.Locale] = struct{}{}
	}
	return true
}

func publicationPaths(slug string, localizations []manualSiteLocalization) []string {
	paths := []string{"/"}
	for _, localization := range localizations {
		if localization.Locale == "zh-CN" {
			paths = append(paths, "/sites/"+slug)
			continue
		}
		paths = append(paths, "/"+localization.Locale, "/"+localization.Locale+"/sites/"+slug)
	}
	return paths
}
