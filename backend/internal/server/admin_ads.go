package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/ad"
	"github.com/forecho/6url/backend/ent/adapplication"
	"github.com/forecho/6url/backend/ent/adlocalization"
	"github.com/forecho/6url/backend/ent/asset"
	"github.com/forecho/6url/backend/ent/paymentorder"
	"github.com/forecho/6url/backend/internal/siteurl"
)

var contentHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type assetRegisterRequest struct {
	ExternalID  string `json:"external_id" binding:"required"`
	StorageKey  string `json:"storage_key" binding:"required"`
	PublicURL   string `json:"public_url" binding:"required"`
	ContentHash string `json:"content_hash" binding:"required"`
	MimeType    string `json:"mime_type" binding:"required"`
	SizeBytes   int64  `json:"size_bytes" binding:"required"`
	Width       int    `json:"width" binding:"required"`
	Height      int    `json:"height" binding:"required"`
	Album       string `json:"album" binding:"required"`
}

type manualAdRequest struct {
	TargetURL     string                 `json:"target_url" binding:"required"`
	ImageAssetID  int                    `json:"image_asset_id" binding:"required"`
	StartsAt      *time.Time             `json:"starts_at"`
	EndsAt        *time.Time             `json:"ends_at"`
	SortOrder     int                    `json:"sort_order"`
	Localizations []manualAdLocalization `json:"localizations" binding:"required,min=1"`
}

type manualAdLocalization struct {
	Locale      string `json:"locale" binding:"required"`
	Title       string `json:"title" binding:"required"`
	Description string `json:"description" binding:"required"`
	CTALabel    string `json:"cta_label" binding:"required"`
	AltText     string `json:"alt_text" binding:"required"`
}

type publishAdRequest struct {
	ImageAssetID  int                    `json:"image_asset_id" binding:"required"`
	SortOrder     int                    `json:"sort_order"`
	Localizations []manualAdLocalization `json:"localizations" binding:"required,len=4"`
}

type commitAdRequest struct {
	Status    string     `json:"status" binding:"required"`
	SortOrder int        `json:"sort_order"`
	StartsAt  *time.Time `json:"starts_at"`
	EndsAt    *time.Time `json:"ends_at"`
	Version   int        `json:"version" binding:"required"`
}

func getAdminAd(c *gin.Context, deps Dependencies) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	row, err := deps.Ent.Ad.Get(c.Request.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			writeError(c, http.StatusNotFound, "not_found", "The advertisement does not exist.")
		} else {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the advertisement.")
		}
		return
	}
	localizations, err := deps.Ent.AdLocalization.Query().Where(adlocalization.AdIDEQ(row.ID)).Order(ent.Asc(adlocalization.FieldLocale)).All(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load advertisement localizations.")
		return
	}
	c.Header("ETag", adETag(row.ID, row.Version))
	c.JSON(http.StatusOK, gin.H{"ad": row, "localizations": localizations})
}

func registerImgAntAsset(c *gin.Context, deps Dependencies) {
	var request assetRegisterRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "The asset payload is invalid.")
		return
	}
	request.Album = strings.TrimSpace(request.Album)
	if request.Album != deps.Config.ImgAntAlbum || !validImgAntURL(request.PublicURL, deps.Config.ImgAntAllowedHosts) || !contentHashPattern.MatchString(strings.ToLower(strings.TrimSpace(request.ContentHash))) || !strings.HasPrefix(strings.ToLower(request.MimeType), "image/") || request.SizeBytes <= 0 || request.Width <= 0 || request.Height <= 0 || !validTrimmedLength(request.ExternalID, 255) || !validTrimmedLength(request.StorageKey, 500) {
		writeError(c, http.StatusBadRequest, "invalid_asset", "The ImgAnt asset metadata is invalid.")
		return
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to register the asset.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	row, err := tx.Asset.Create().SetProvider("imgant").SetAlbum(request.Album).SetExternalID(strings.TrimSpace(request.ExternalID)).SetStorageKey(strings.TrimSpace(request.StorageKey)).SetPublicURL(strings.TrimSpace(request.PublicURL)).SetContentHash(strings.ToLower(strings.TrimSpace(request.ContentHash))).SetMimeType(strings.ToLower(strings.TrimSpace(request.MimeType))).SetSizeBytes(request.SizeBytes).SetWidth(request.Width).SetHeight(request.Height).SetStatus(asset.StatusHealthy).SetCreatedByTokenID(adminIdentity(c).ID).Save(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, "asset_conflict", "The ImgAnt asset is already registered or could not be stored.")
		return
	}
	if _, err := tx.AdminAuditLog.Create().SetAdminAPITokenID(adminIdentity(c).ID).SetAction("asset.register").SetResourceType("asset").SetResourceID(strconv.Itoa(row.ID)).SetReason("registered ImgAnt 6url album asset").Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to write the asset audit log.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to register the asset.")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": row.ID, "provider": row.Provider, "album": row.Album, "status": row.Status, "public_url": row.PublicURL})
}

func createManualAd(c *gin.Context, deps Dependencies) {
	var request manualAdRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.ImageAssetID <= 0 {
		writeError(c, http.StatusBadRequest, "invalid_request", "The sponsor advertisement payload is invalid.")
		return
	}
	target, err := siteurl.Normalize(request.TargetURL)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_url", "target_url must be an absolute public HTTP or HTTPS URL.")
		return
	}
	if request.StartsAt != nil && request.EndsAt != nil && !request.EndsAt.After(*request.StartsAt) {
		writeError(c, http.StatusBadRequest, "invalid_schedule", "ends_at must be later than starts_at.")
		return
	}
	validAsset, err := deps.Ent.Asset.Query().Where(asset.IDEQ(request.ImageAssetID), asset.ProviderEQ("imgant"), asset.AlbumEQ("6url"), asset.StatusEQ(asset.StatusHealthy)).Exist(c.Request.Context())
	if err != nil || !validAsset {
		writeError(c, http.StatusBadRequest, "invalid_asset", "image_asset_id must reference a healthy ImgAnt asset in the 6url album.")
		return
	}
	seen := make(map[string]struct{}, len(request.Localizations))
	for _, localization := range request.Localizations {
		if _, supported := supportedApplicationLocales[localization.Locale]; !supported || !validTrimmedLength(localization.Title, 180) || !validTrimmedLength(localization.Description, 500) || !validTrimmedLength(localization.CTALabel, 80) || !validTrimmedLength(localization.AltText, 300) {
			writeError(c, http.StatusBadRequest, "invalid_localization", "Every advertisement localization must use a supported locale and valid content lengths.")
			return
		}
		if _, duplicate := seen[localization.Locale]; duplicate {
			writeError(c, http.StatusBadRequest, "duplicate_locale", "Each advertisement locale may appear only once.")
			return
		}
		seen[localization.Locale] = struct{}{}
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create the sponsor advertisement.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	creator := tx.Ad.Create().SetTargetURL(target.URL).SetImageAssetID(request.ImageAssetID).SetSortOrder(request.SortOrder).SetOrigin(ad.OriginAdmin).SetCreatedByTokenID(adminIdentity(c).ID).SetStatus(ad.StatusDraft)
	if request.StartsAt != nil {
		creator.SetStartsAt(*request.StartsAt)
	}
	if request.EndsAt != nil {
		creator.SetEndsAt(*request.EndsAt)
	}
	row, err := creator.Save(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create the sponsor advertisement.")
		return
	}
	for _, localization := range request.Localizations {
		if _, err := tx.AdLocalization.Create().SetAdID(row.ID).SetLocale(localization.Locale).SetTitle(strings.TrimSpace(localization.Title)).SetDescription(strings.TrimSpace(localization.Description)).SetCtaLabel(strings.TrimSpace(localization.CTALabel)).SetAltText(strings.TrimSpace(localization.AltText)).Save(c.Request.Context()); err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create advertisement localizations.")
			return
		}
	}
	if _, err := tx.AdminAuditLog.Create().SetAdminAPITokenID(adminIdentity(c).ID).SetAction("ad.manual_create").SetResourceType("ad").SetResourceID(strconv.Itoa(row.ID)).SetReason("administrator free sponsor advertisement").Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to write the sponsor advertisement audit log.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create the sponsor advertisement.")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": row.ID, "status": row.Status, "origin": row.Origin, "payment_order_id": nil})
}

func publishAdApplication(c *gin.Context, deps Dependencies) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var request publishAdRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.ImageAssetID <= 0 || !validateCompleteAdLocalizations(request.Localizations) {
		writeError(c, http.StatusBadRequest, "invalid_request", "Publication requires a healthy image and one valid localization for each supported locale.")
		return
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to publish the sponsor advertisement.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	validAsset, err := tx.Asset.Query().Where(asset.IDEQ(request.ImageAssetID), asset.ProviderEQ("imgant"), asset.AlbumEQ("6url"), asset.StatusEQ(asset.StatusHealthy)).Exist(c.Request.Context())
	if err != nil || !validAsset {
		writeError(c, http.StatusBadRequest, "invalid_asset", "image_asset_id must reference a healthy ImgAnt asset in the 6url album.")
		return
	}
	application, err := tx.AdApplication.Get(c.Request.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			writeError(c, http.StatusNotFound, "not_found", "The advertising application does not exist.")
		} else {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the advertising application.")
		}
		return
	}
	if application.Status != adapplication.StatusPaid {
		writeError(c, http.StatusConflict, "invalid_state", "The advertising application is not ready for publication.")
		return
	}
	order, err := tx.PaymentOrder.Query().Where(paymentorder.AdApplicationIDEQ(application.ID), paymentorder.PurposeEQ(paymentorder.PurposeAd), paymentorder.StatusEQ(paymentorder.StatusPaid)).Only(c.Request.Context())
	if err != nil || order.DurationDaysSnapshot == nil || *order.DurationDaysSnapshot <= 0 || !sameLocaleSet(order.LocalesSnapshot, localizationCodes(request.Localizations)) {
		writeError(c, http.StatusConflict, "invalid_order", "The paid sponsor order or its locale entitlement is invalid.")
		return
	}
	exists, err := tx.Ad.Query().Where(ad.AdApplicationIDEQ(application.ID)).Exist(c.Request.Context())
	if err != nil || exists {
		writeError(c, http.StatusConflict, "ad_exists", "This advertising application already has an advertisement.")
		return
	}
	now := time.Now().UTC()
	endsAt := now.AddDate(0, 0, *order.DurationDaysSnapshot)
	row, err := tx.Ad.Create().SetAdApplicationID(application.ID).SetPaymentOrderID(order.ID).SetStatus(ad.StatusActive).SetTargetURL(application.TargetURL).SetImageAssetID(request.ImageAssetID).SetStartsAt(now).SetEndsAt(endsAt).SetSortOrder(request.SortOrder).SetOrigin(ad.OriginAdvertiser).SetCreatedByTokenID(adminIdentity(c).ID).Save(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to create the sponsor advertisement.")
		return
	}
	for _, localization := range request.Localizations {
		if _, err := tx.AdLocalization.Create().SetAdID(row.ID).SetLocale(localization.Locale).SetTitle(strings.TrimSpace(localization.Title)).SetDescription(strings.TrimSpace(localization.Description)).SetCtaLabel(strings.TrimSpace(localization.CTALabel)).SetAltText(strings.TrimSpace(localization.AltText)).Save(c.Request.Context()); err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to publish advertisement localizations.")
			return
		}
	}
	changed, err := tx.AdApplication.Update().Where(adapplication.IDEQ(application.ID), adapplication.StatusEQ(adapplication.StatusPaid)).SetStatus(adapplication.StatusPublished).SetImageAssetID(request.ImageAssetID).Save(c.Request.Context())
	if err != nil || changed != 1 {
		writeError(c, http.StatusConflict, "concurrent_update", "The advertising application changed while it was being published.")
		return
	}
	diff, _ := json.Marshal(map[string]any{"from": adapplication.StatusPaid, "to": adapplication.StatusPublished, "ad_id": row.ID, "ends_at": endsAt})
	if _, err := tx.AdminAuditLog.Create().SetAdminAPITokenID(adminIdentity(c).ID).SetAction("ad_application.publish").SetResourceType("ad_application").SetResourceID(strconv.Itoa(application.ID)).SetDiffSummaryJSON(string(diff)).SetReason("published paid sponsor advertisement").Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to write the advertisement audit log.")
		return
	}
	paths := adHomepagePaths()
	if _, err := tx.RevalidationJob.Create().SetDedupeKey("ad:publish:" + strconv.Itoa(row.ID) + ":" + strconv.FormatInt(now.UnixNano(), 36)).SetPaths(paths).Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to queue frontend revalidation.")
		return
	}
	payload := notificationPayload{ApplicantName: application.ApplicantName, Email: application.ApplicantEmail, ResourceID: application.ID, ResourceType: "ad_application", Name: application.RawTitle, URL: application.TargetURL, Status: string(adapplication.StatusPublished), PageURL: localizedHomepageURL(deps.Config.AppBaseURL, application.PreferredLocale), StartsAt: &now, EndsAt: &endsAt}
	if err := createApplicationOutbox(c, deps, tx, "ad_application.published", application.ID, application.PreferredLocale, application.ApplicantEmail, payload); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to queue the advertisement publication notification.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to publish the sponsor advertisement.")
		return
	}
	c.Header("ETag", adETag(row.ID, row.Version))
	c.JSON(http.StatusOK, gin.H{"ad_application_id": application.ID, "ad_id": row.ID, "payment_order_id": order.ID, "status": row.Status, "starts_at": now, "ends_at": endsAt, "paths": paths, "version": row.Version})
}

func commitAd(c *gin.Context, deps Dependencies) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var request commitAdRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Version <= 0 {
		writeError(c, http.StatusBadRequest, "invalid_request", "The advertisement commit payload is invalid.")
		return
	}
	status := ad.Status(strings.TrimSpace(request.Status))
	if status != ad.StatusActive && status != ad.StatusPaused && status != ad.StatusCanceled {
		writeError(c, http.StatusBadRequest, "invalid_status", "status must be active, paused, or canceled.")
		return
	}
	if request.StartsAt != nil && request.EndsAt != nil && !request.EndsAt.After(*request.StartsAt) {
		writeError(c, http.StatusBadRequest, "invalid_schedule", "ends_at must be later than starts_at.")
		return
	}
	tx, err := deps.Ent.Tx(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to commit the advertisement.")
		return
	}
	defer func() { _ = tx.Rollback() }()
	current, err := tx.Ad.Get(c.Request.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			writeError(c, http.StatusNotFound, "not_found", "The advertisement does not exist.")
		} else {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the advertisement.")
		}
		return
	}
	if current.Version != request.Version || !matchesIfMatch(c.GetHeader("If-Match"), adETag(id, request.Version)) {
		writeError(c, http.StatusPreconditionFailed, "version_mismatch", "The advertisement version or If-Match value is stale.")
		return
	}
	if current.Status == ad.StatusExpired || current.Status == ad.StatusCanceled {
		writeError(c, http.StatusConflict, "invalid_state", "An expired or canceled advertisement cannot be committed.")
		return
	}
	if request.StartsAt != nil && request.EndsAt == nil && current.Origin == ad.OriginAdvertiser && current.EndsAt != nil && !current.EndsAt.After(*request.StartsAt) {
		writeError(c, http.StatusBadRequest, "invalid_schedule", "starts_at must be earlier than the paid advertisement end time.")
		return
	}
	if status == ad.StatusActive {
		validAsset, err := tx.Asset.Query().Where(asset.IDEQ(current.ImageAssetID), asset.ProviderEQ("imgant"), asset.AlbumEQ("6url"), asset.StatusEQ(asset.StatusHealthy)).Exist(c.Request.Context())
		localizations, localeErr := tx.AdLocalization.Query().Where(adlocalization.AdIDEQ(current.ID)).All(c.Request.Context())
		if err != nil || !validAsset || localeErr != nil || !hasCompleteAdLocaleCoverage(localizations) {
			writeError(c, http.StatusConflict, "publication_incomplete", "An active advertisement requires a healthy image and all supported localizations.")
			return
		}
	}
	updater := tx.Ad.Update().Where(ad.IDEQ(id), ad.VersionEQ(request.Version), ad.StatusEQ(current.Status)).SetStatus(status).SetSortOrder(request.SortOrder).AddVersion(1)
	if request.StartsAt != nil {
		updater.SetStartsAt(*request.StartsAt)
	} else {
		updater.ClearStartsAt()
	}
	if request.EndsAt != nil {
		updater.SetEndsAt(*request.EndsAt)
	} else if current.Origin == ad.OriginAdmin {
		updater.ClearEndsAt()
	}
	changed, err := updater.Save(c.Request.Context())
	if err != nil || changed != 1 {
		writeError(c, http.StatusConflict, "concurrent_update", "The advertisement changed while it was being committed.")
		return
	}
	diff, _ := json.Marshal(map[string]any{"from": current.Status, "to": status, "sort_order": request.SortOrder, "starts_at": request.StartsAt, "ends_at": request.EndsAt})
	if _, err := tx.AdminAuditLog.Create().SetAdminAPITokenID(adminIdentity(c).ID).SetAction("ad.commit").SetResourceType("ad").SetResourceID(strconv.Itoa(id)).SetDiffSummaryJSON(string(diff)).SetReason("committed sponsor advertisement state").Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to write the advertisement audit log.")
		return
	}
	paths := adHomepagePaths()
	now := time.Now().UTC()
	if _, err := tx.RevalidationJob.Create().SetDedupeKey("ad:commit:" + strconv.Itoa(id) + ":" + strconv.FormatInt(now.UnixNano(), 36)).SetPaths(paths).Save(c.Request.Context()); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to queue frontend revalidation.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to commit the advertisement.")
		return
	}
	c.Header("ETag", adETag(id, request.Version+1))
	c.JSON(http.StatusOK, gin.H{"id": id, "status": status, "version": request.Version + 1, "paths": paths})
}

func validateCompleteAdLocalizations(values []manualAdLocalization) bool {
	if len(values) != len(supportedApplicationLocales) {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, supported := supportedApplicationLocales[value.Locale]; !supported || !validTrimmedLength(value.Title, 180) || !validTrimmedLength(value.Description, 500) || !validTrimmedLength(value.CTALabel, 80) || !validTrimmedLength(value.AltText, 300) {
			return false
		}
		if _, duplicate := seen[value.Locale]; duplicate {
			return false
		}
		seen[value.Locale] = struct{}{}
	}
	return true
}

func hasCompleteAdLocaleCoverage(values []*ent.AdLocalization) bool {
	if len(values) != len(supportedApplicationLocales) {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, supported := supportedApplicationLocales[value.Locale]; !supported {
			return false
		}
		seen[value.Locale] = struct{}{}
	}
	return len(seen) == len(supportedApplicationLocales)
}

func localizationCodes(values []manualAdLocalization) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.Locale)
	}
	return result
}

func sameLocaleSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	values := make(map[string]struct{}, len(left))
	for _, value := range left {
		values[value] = struct{}{}
	}
	if len(values) != len(left) {
		return false
	}
	for _, value := range right {
		if _, ok := values[value]; !ok {
			return false
		}
	}
	return true
}

func adHomepagePaths() []string { return []string{"/", "/en", "/ja", "/ko"} }

func localizedHomepageURL(baseURL, localeCode string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if localeCode == "zh-CN" {
		return baseURL + "/"
	}
	return baseURL + "/" + localeCode
}

func adETag(id, version int) string {
	return `W/"ad-` + strconv.Itoa(id) + `-v` + strconv.Itoa(version) + `"`
}

func matchesIfMatch(value, expected string) bool { return strings.TrimSpace(value) == expected }

func validImgAntURL(raw string, allowedHosts []string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, allowed := range allowedHosts {
		if host == strings.ToLower(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}
