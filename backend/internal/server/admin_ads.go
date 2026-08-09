package server

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/forecho/6url/backend/ent/ad"
	"github.com/forecho/6url/backend/ent/asset"
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
