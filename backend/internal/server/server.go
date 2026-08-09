package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/locale"
	"github.com/forecho/6url/backend/internal/backlink"
	"github.com/forecho/6url/backend/internal/catalog"
	"github.com/forecho/6url/backend/internal/config"
	"github.com/forecho/6url/backend/internal/ratelimit"
	"github.com/forecho/6url/backend/internal/securepayload"
	"github.com/forecho/6url/backend/internal/stripeclient"
	"github.com/forecho/6url/backend/internal/turnstile"
)

type StripeCheckout interface {
	CreateCheckout(context.Context, stripeclient.CheckoutRequest) (stripeclient.CheckoutSession, error)
}

type StripeRefund interface {
	Refund(context.Context, stripeclient.RefundRequest) (stripeclient.Refund, error)
}

type Dependencies struct {
	Config        config.Config
	Ent           *ent.Client
	SQL           *sql.DB
	Turnstile     turnstile.Verifier
	RateLimiter   ratelimit.Limiter
	PayloadSealer *securepayload.Sealer
	Stripe        StripeCheckout
	StripeRefund  StripeRefund
	Backlinks     backlink.Checker
}

func New(deps Dependencies) *gin.Engine {
	gin.SetMode(deps.Config.GinMode)
	router := gin.New()
	router.Use(gin.Recovery())
	if err := router.SetTrustedProxies(deps.Config.TrustedProxyCIDRs); err != nil {
		panic("invalid trusted proxy configuration: " + err.Error())
	}
	if deps.Turnstile == nil {
		deps.Turnstile = turnstile.Client{SecretKey: deps.Config.TurnstileSecretKey}
	}
	if deps.RateLimiter == nil {
		deps.RateLimiter = ratelimit.NewMemory()
	}
	if deps.PayloadSealer == nil && deps.Config.NotificationEncryptionKey != "" {
		if sealer, err := securepayload.New(deps.Config.NotificationEncryptionKey); err == nil {
			deps.PayloadSealer = &sealer
		}
	}
	if deps.Stripe == nil {
		deps.Stripe = stripeclient.Client{SecretKey: deps.Config.StripeSecretKey}
	}
	if deps.StripeRefund == nil {
		deps.StripeRefund = stripeclient.Client{SecretKey: deps.Config.StripeSecretKey}
	}
	if deps.Backlinks == nil {
		deps.Backlinks = backlink.Client{}
	}
	router.Use(cors.New(cors.Config{
		AllowOrigins:     deps.Config.CORSAllowedOrigins,
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"Authorization", "Content-Type", "If-Match", "Idempotency-Key", "X-CSRF-Token"},
		ExposeHeaders:    []string{"ETag", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := contextWithTimeout(c, 2*time.Second)
		defer cancel()
		if err := deps.SQL.PingContext(ctx); err != nil {
			writeError(c, http.StatusServiceUnavailable, "database_unavailable", "The database is unavailable.")
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	public := router.Group("/api/v1")
	registerPublicApplicationRoutes(public, deps)
	public.GET("/locales", func(c *gin.Context) {
		rows, err := deps.Ent.Locale.Query().Where(locale.EnabledEQ(true)).Order(ent.Asc(locale.FieldSortOrder)).All(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load locales.")
			return
		}
		items := make([]gin.H, 0, len(rows))
		for _, row := range rows {
			items = append(items, gin.H{"code": row.Code, "name": row.Name, "native_name": row.NativeName, "is_default": row.IsDefault})
		}
		c.JSON(http.StatusOK, gin.H{"data": items})
	})
	public.GET("/catalog", func(c *gin.Context) {
		localeCode := strings.TrimSpace(c.DefaultQuery("locale", "zh-CN"))
		valid, err := deps.Ent.Locale.Query().Where(locale.CodeEQ(localeCode), locale.EnabledEQ(true)).Exist(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the catalog.")
			return
		}
		if !valid {
			writeError(c, http.StatusBadRequest, "unsupported_locale", "The requested locale is not supported.")
			return
		}
		result, err := (catalog.Service{Client: deps.Ent}).Load(c.Request.Context(), localeCode)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the catalog.")
			return
		}
		writeJSONWithETag(c, result)
	})

	auth := adminAuthenticator{client: deps.Ent, pepper: deps.Config.AdminTokenPepper, now: time.Now}
	admin := router.Group("/admin/v1")
	admin.Use(auth.middleware(""))
	admin.GET("/me", func(c *gin.Context) {
		identity := adminIdentity(c)
		scopes := append([]string(nil), identity.Scopes...)
		sort.Strings(scopes)
		c.JSON(http.StatusOK, gin.H{"id": identity.ID, "name": identity.Name, "prefix": identity.Prefix, "scopes": scopes, "expires_at": identity.ExpiresAt})
	})
	admin.GET("/overview", requireScope("catalog:read"), func(c *gin.Context) {
		ctx := c.Request.Context()
		categories, err := deps.Ent.Category.Query().Count(ctx)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the administrator overview.")
			return
		}
		sites, err := deps.Ent.Site.Query().Count(ctx)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the administrator overview.")
			return
		}
		submissions, err := deps.Ent.Submission.Query().Count(ctx)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the administrator overview.")
			return
		}
		adApplications, err := deps.Ent.AdApplication.Query().Count(ctx)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the administrator overview.")
			return
		}
		ads, err := deps.Ent.Ad.Query().Count(ctx)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Unable to load the administrator overview.")
			return
		}
		c.JSON(http.StatusOK, gin.H{"categories": categories, "sites": sites, "submissions": submissions, "ad_applications": adApplications, "ads": ads})
	})
	registerAdminRoutes(admin, deps)

	return router
}

func writeJSONWithETag(c *gin.Context, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "Unable to encode the response.")
		return
	}
	digest := sha256.Sum256(payload)
	etag := `W/"` + hex.EncodeToString(digest[:16]) + `"`
	c.Header("ETag", etag)
	c.Header("Cache-Control", "public, max-age=60, s-maxage=300, stale-while-revalidate=86400")
	if c.GetHeader("If-None-Match") == etag {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
}
