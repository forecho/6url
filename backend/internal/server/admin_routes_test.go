package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"

	"github.com/forecho/6url/backend/ent/category"
	"github.com/forecho/6url/backend/ent/enttest"
	"github.com/forecho/6url/backend/ent/placement"
	"github.com/forecho/6url/backend/internal/admintoken"
	"github.com/forecho/6url/backend/internal/config"
)

func TestManualSiteCreatesEditorialPlacementWithoutPayment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:manual-site?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	client.Locale.Create().SetCode("zh-CN").SetName("Chinese").SetNativeName("简体中文").SetIsDefault(true).SaveX(ctx)
	categoryRow := client.Category.Create().SetSlug("tools").SetStatus(category.StatusDraft).SaveX(ctx)
	pepper := "01234567890123456789012345678901"
	raw, prefix, err := admintoken.Generate()
	if err != nil {
		t.Fatal(err)
	}
	client.AdminAPIToken.Create().SetName("test").SetPrefix(prefix).SetTokenHash(admintoken.Hash(raw, pepper)).SetScopes([]string{"catalog:read", "content:write"}).SaveX(ctx)
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, CORSAllowedOrigins: []string{"https://6url.net"}, AdminTokenPepper: pepper}, Ent: client})
	body := map[string]any{
		"canonical_url": "https://www.example.com/product#details",
		"slug":          "example-product",
		"category_id":   categoryRow.ID,
		"localizations": []map[string]any{{"locale": "zh-CN", "name": "Example", "tagline": "示例", "summary": "摘要", "description": "详细介绍", "seo_title": "Example", "seo_description": "Example description"}},
	}
	payload, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, "/admin/v1/sites/manual", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+raw)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := client.PaymentOrder.Query().CountX(ctx); got != 0 {
		t.Fatalf("payment order count = %d", got)
	}
	if got := client.Submission.Query().CountX(ctx); got != 0 {
		t.Fatalf("submission count = %d", got)
	}
	placementRow := client.Placement.Query().OnlyX(ctx)
	if placementRow.Kind != placement.KindEditorial || placementRow.Origin != placement.OriginAdmin {
		t.Fatalf("unexpected placement: %#v", placementRow)
	}
	if got := client.AdminAuditLog.Query().CountX(ctx); got != 1 {
		t.Fatalf("audit log count = %d", got)
	}
	siteRow := client.Site.Query().OnlyX(ctx)
	if siteRow.RegistrableDomain != "example.com" || len(siteRow.CanonicalURLHash) != 64 {
		t.Fatalf("unexpected normalized site identity: %#v", siteRow)
	}
}

func TestPaymentReadTokenDoesNotRequireCatalogScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:payment-scope?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	pepper := "01234567890123456789012345678901"
	raw, prefix, err := admintoken.Generate()
	if err != nil {
		t.Fatal(err)
	}
	client.AdminAPIToken.Create().SetName("finance").SetPrefix(prefix).SetTokenHash(admintoken.Hash(raw, pepper)).SetScopes([]string{"payments:read"}).SaveX(ctx)
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, CORSAllowedOrigins: []string{"https://6url.net"}, AdminTokenPepper: pepper}, Ent: client})
	request := httptest.NewRequest(http.MethodGet, "/admin/v1/orders", nil)
	request.Header.Set("Authorization", "Bearer "+raw)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestManualSiteRejectsUnsafeSlugBeforeWriting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:invalid-slug?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	client.Locale.Create().SetCode("zh-CN").SetName("Chinese").SetNativeName("简体中文").SetIsDefault(true).SaveX(ctx)
	categoryRow := client.Category.Create().SetSlug("tools").SetStatus(category.StatusDraft).SaveX(ctx)
	pepper := "01234567890123456789012345678901"
	raw, prefix, err := admintoken.Generate()
	if err != nil {
		t.Fatal(err)
	}
	client.AdminAPIToken.Create().SetName("test").SetPrefix(prefix).SetTokenHash(admintoken.Hash(raw, pepper)).SetScopes([]string{"content:write"}).SaveX(ctx)
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, CORSAllowedOrigins: []string{"https://6url.net"}, AdminTokenPepper: pepper}, Ent: client})
	body := map[string]any{
		"canonical_url": "https://example.com",
		"slug":          "unsafe/path",
		"category_id":   categoryRow.ID,
		"localizations": []map[string]any{{"locale": "zh-CN", "name": "Example", "tagline": "示例", "summary": "摘要", "description": "详细介绍", "seo_title": "Example", "seo_description": "Example description"}},
	}
	payload, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, "/admin/v1/sites/manual", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+raw)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := client.Site.Query().CountX(ctx); got != 0 {
		t.Fatalf("site count = %d", got)
	}
}
