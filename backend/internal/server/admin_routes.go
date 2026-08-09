package server

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/ad"
	"github.com/forecho/6url/backend/ent/adapplication"
	"github.com/forecho/6url/backend/ent/asset"
	"github.com/forecho/6url/backend/ent/category"
	"github.com/forecho/6url/backend/ent/locale"
	"github.com/forecho/6url/backend/ent/paymentorder"
	"github.com/forecho/6url/backend/ent/placement"
	"github.com/forecho/6url/backend/ent/site"
	"github.com/forecho/6url/backend/ent/sitecontentrevision"
	"github.com/forecho/6url/backend/ent/sitelocalepublication"
	"github.com/forecho/6url/backend/ent/submission"
	"github.com/forecho/6url/backend/internal/siteurl"
)

const maxAdminPageSize = 100

var validSlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type manualSiteRequest struct {
	CanonicalURL string                   `json:"canonical_url" binding:"required"`
	Slug         string                   `json:"slug" binding:"required"`
	CategoryID   int                      `json:"category_id" binding:"required"`
	LogoAssetID  *int                     `json:"logo_asset_id"`
	Locales      []manualSiteLocalization `json:"localizations" binding:"required,min=1"`
}

type manualSiteLocalization struct {
	Locale         string `json:"locale" binding:"required"`
	Name           string `json:"name" binding:"required"`
	Tagline        string `json:"tagline" binding:"required"`
	Summary        string `json:"summary" binding:"required"`
	Description    string `json:"description" binding:"required"`
	SeoTitle       string `json:"seo_title" binding:"required"`
	SeoDescription string `json:"seo_description" binding:"required"`
}

func registerAdminRoutes(admin *gin.RouterGroup, deps Dependencies) {
	admin.Use(limitRequestBody(512 << 10))
	admin.GET("/openapi.json", serveAdminOpenAPI)
	admin.GET("/categories", requireScope("catalog:read"), func(c *gin.Context) {
		rows, err := deps.Ent.Category.Query().Order(ent.Asc(category.FieldSortOrder), ent.Asc(category.FieldID)).Limit(pageSize(c)).All(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load categories.")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": rows})
	})
	admin.GET("/sites", requireScope("catalog:read"), func(c *gin.Context) {
		query := deps.Ent.Site.Query().Order(ent.Desc(site.FieldUpdatedAt), ent.Desc(site.FieldID)).Limit(pageSize(c))
		if value := strings.TrimSpace(c.Query("status")); value != "" {
			query = query.Where(site.StatusEQ(site.Status(value)))
		}
		rows, err := query.All(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load sites.")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": rows})
	})
	admin.GET("/submissions", requireScope("submissions:review"), func(c *gin.Context) {
		rows, err := deps.Ent.Submission.Query().Order(ent.Desc(submission.FieldSubmittedAt), ent.Desc(submission.FieldID)).Limit(pageSize(c)).All(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load submissions.")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": rows})
	})
	admin.GET("/ad-applications", requireScope("ad_applications:review"), func(c *gin.Context) {
		rows, err := deps.Ent.AdApplication.Query().Order(ent.Desc(adapplication.FieldCreatedAt), ent.Desc(adapplication.FieldID)).Limit(pageSize(c)).All(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load advertising applications.")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": rows})
	})
	admin.GET("/ads", requireScope("ads:write"), func(c *gin.Context) {
		rows, err := deps.Ent.Ad.Query().Order(ent.Asc(ad.FieldSortOrder), ent.Asc(ad.FieldID)).Limit(pageSize(c)).All(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load advertisements.")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": rows})
	})
	admin.GET("/ads/:id", requireScope("ads:write"), func(c *gin.Context) { getAdminAd(c, deps) })
	admin.GET("/orders", requireScope("payments:read"), func(c *gin.Context) {
		rows, err := deps.Ent.PaymentOrder.Query().Order(ent.Desc(paymentorder.FieldCreatedAt), ent.Desc(paymentorder.FieldID)).Limit(pageSize(c)).All(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load payment orders.")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": rows})
	})
	admin.POST("/sites/manual", requireScope("content:write"), func(c *gin.Context) {
		createManualSite(c, deps)
	})
	admin.POST("/assets/register", requireScope("assets:write"), func(c *gin.Context) { registerImgAntAsset(c, deps) })
	admin.POST("/ads/manual", requireScope("ads:write"), func(c *gin.Context) { createManualAd(c, deps) })
	admin.POST("/ads/:id/commit", requireScope("ads:write"), func(c *gin.Context) { commitAd(c, deps) })
	admin.POST("/submissions/:id/preapprove", requireScope("submissions:review"), func(c *gin.Context) { reviewSubmission(c, deps, "preapprove") })
	admin.POST("/submissions/:id/request-changes", requireScope("submissions:review"), func(c *gin.Context) { reviewSubmission(c, deps, "request_changes") })
	admin.POST("/submissions/:id/reject", requireScope("submissions:review"), func(c *gin.Context) { reviewSubmission(c, deps, "reject") })
	admin.POST("/submissions/:id/publish", requireScope("content:publish"), func(c *gin.Context) { publishSubmission(c, deps) })
	admin.POST("/placements/:id/recheck", requireScope("placements:write"), func(c *gin.Context) { recheckAdminPlacement(c, deps) })
	admin.POST("/ad-applications/:id/preapprove", requireScope("ad_applications:review"), func(c *gin.Context) { reviewAdApplication(c, deps, "preapprove") })
	admin.POST("/ad-applications/:id/request-changes", requireScope("ad_applications:review"), func(c *gin.Context) { reviewAdApplication(c, deps, "request_changes") })
	admin.POST("/ad-applications/:id/reject", requireScope("ad_applications:review"), func(c *gin.Context) { reviewAdApplication(c, deps, "reject") })
	admin.POST("/ad-applications/:id/publish", requireScope("ads:write"), func(c *gin.Context) { publishAdApplication(c, deps) })
	admin.POST("/orders/:id/mark-paid", requireScope("payments:write"), func(c *gin.Context) { markManualOrderPaid(c, deps) })
	admin.POST("/orders/:id/refund", requireScope("payments:write"), func(c *gin.Context) { refundOrder(c, deps) })
}

func createManualSite(c *gin.Context, deps Dependencies) {
	var request manualSiteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "The site payload is invalid.")
		return
	}
	normalized, err := siteurl.Normalize(request.CanonicalURL)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_url", "canonical_url must be an absolute public HTTP or HTTPS URL.")
		return
	}
	slug := strings.TrimSpace(request.Slug)
	if len(slug) > 180 || !validSlug.MatchString(slug) {
		writeError(c, http.StatusBadRequest, "invalid_slug", "slug must use lowercase ASCII letters, numbers, and single hyphens.")
		return
	}
	if _, err := deps.Ent.Category.Query().Where(category.IDEQ(request.CategoryID)).Only(c.Request.Context()); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_category", "The selected category does not exist.")
		return
	}
	if request.LogoAssetID != nil {
		valid, err := deps.Ent.Asset.Query().Where(asset.IDEQ(*request.LogoAssetID), asset.ProviderEQ("imgant"), asset.AlbumEQ("6url"), asset.StatusEQ(asset.StatusHealthy)).Exist(c.Request.Context())
		if err != nil || !valid {
			writeError(c, http.StatusBadRequest, "invalid_asset", "The logo must be a healthy ImgAnt asset in the 6url album.")
			return
		}
	}
	seenLocales := make(map[string]struct{}, len(request.Locales))
	for _, localization := range request.Locales {
		if !validManualLocalization(localization) {
			writeError(c, http.StatusBadRequest, "invalid_localization", "Localized content is empty or exceeds its field limit.")
			return
		}
		if _, exists := seenLocales[localization.Locale]; exists {
			writeError(c, http.StatusBadRequest, "duplicate_locale", "Each localization may appear only once.")
			return
		}
		seenLocales[localization.Locale] = struct{}{}
		valid, err := deps.Ent.Locale.Query().Where(locale.CodeEQ(localization.Locale), locale.EnabledEQ(true)).Exist(c.Request.Context())
		if err != nil || !valid {
			writeError(c, http.StatusBadRequest, "unsupported_locale", "Every localization must use an enabled locale.")
			return
		}
	}

	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create the site.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	creator := tx.Site.Create().SetCanonicalURL(normalized.URL).SetCanonicalURLHash(normalized.Hash).SetRegistrableDomain(normalized.RegistrableDomain).SetSlug(slug).SetStatus(site.StatusDraft).SetCreatedVia(site.CreatedViaAdminManual)
	if request.LogoAssetID != nil {
		creator.SetLogoAssetID(*request.LogoAssetID)
	}
	row, err := creator.Save(c.Request.Context())
	if err != nil {
		if ent.IsConstraintError(err) {
			writeError(c, http.StatusConflict, "site_conflict", "A site with this URL or slug already exists.")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create the site.")
		return
	}
	for _, localization := range request.Locales {
		revision, err := tx.SiteContentRevision.Create().SetSiteID(row.ID).SetLocale(localization.Locale).SetRevision(1).SetStatus(sitecontentrevision.StatusDraft).SetName(strings.TrimSpace(localization.Name)).SetTagline(strings.TrimSpace(localization.Tagline)).SetSummary(strings.TrimSpace(localization.Summary)).SetDescription(strings.TrimSpace(localization.Description)).SetSeoTitle(strings.TrimSpace(localization.SeoTitle)).SetSeoDescription(strings.TrimSpace(localization.SeoDescription)).SetSourceLocale(localization.Locale).SetCreatedByTokenID(adminIdentity(c).ID).Save(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create localized content.")
			return
		}
		if _, err := tx.SiteLocalePublication.Create().SetSiteID(row.ID).SetLocale(localization.Locale).SetDraftRevisionID(revision.ID).SetTranslationStatus(sitelocalepublication.TranslationStatusDraft).Save(c.Request.Context()); err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create localized content.")
			return
		}
	}
	if _, err := tx.SiteCategory.Create().SetSiteID(row.ID).SetCategoryID(request.CategoryID).SetIsPrimary(true).Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to assign the category.")
		return
	}
	placementRow, err := tx.Placement.Create().SetSiteID(row.ID).SetKind(placement.KindEditorial).SetOrigin(placement.OriginAdmin).SetStatus(placement.StatusActive).SetOutboundURL(normalized.URL).SetOutboundRel("noopener").SetDisclosureKey("editorial").Save(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create the editorial placement.")
		return
	}
	for localeCode := range seenLocales {
		if _, err := tx.PlacementLocale.Create().SetPlacementID(placementRow.ID).SetLocale(localeCode).Save(c.Request.Context()); err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create locale eligibility.")
			return
		}
	}
	if _, err := tx.AdminAuditLog.Create().SetAdminAPITokenID(adminIdentity(c).ID).SetAction("site.manual_create").SetResourceType("site").SetResourceID(strconv.Itoa(row.ID)).SetReason("administrator free listing").Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to write the audit log.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create the site.")
		return
	}
	c.Header("ETag", `W/"site-`+strconv.Itoa(row.ID)+`-v1"`)
	c.JSON(http.StatusCreated, gin.H{"id": row.ID, "version": 1, "status": "draft", "placement_kind": "editorial", "payment_order_id": nil})
}

func validManualLocalization(value manualSiteLocalization) bool {
	return validTrimmedLength(value.Locale, 16) &&
		validTrimmedLength(value.Name, 180) &&
		validTrimmedLength(value.Tagline, 280) &&
		validTrimmedLength(value.Summary, 20_000) &&
		validTrimmedLength(value.Description, 50_000) &&
		validTrimmedLength(value.SeoTitle, 180) &&
		validTrimmedLength(value.SeoDescription, 320)
}

func validTrimmedLength(value string, limit int) bool {
	length := len([]rune(strings.TrimSpace(value)))
	return length > 0 && length <= limit
}

func pageSize(c *gin.Context) int {
	value, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || value <= 0 {
		return 50
	}
	if value > maxAdminPageSize {
		return maxAdminPageSize
	}
	return value
}
