package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/adminapitoken"
	"github.com/forecho/6url/backend/internal/admintoken"
)

type adminIdentityKey struct{}

type adminAuthenticator struct {
	client *ent.Client
	pepper string
	now    func() time.Time
}

func (a adminAuthenticator) authenticate(ctx context.Context, raw string) (*ent.AdminAPIToken, error) {
	prefix, err := admintoken.Prefix(raw)
	if err != nil {
		return nil, err
	}
	row, err := a.client.AdminAPIToken.Query().Where(adminapitoken.PrefixEQ(prefix), adminapitoken.RevokedAtIsNil()).Only(ctx)
	if err != nil {
		return nil, errors.New("invalid admin token")
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(a.now()) {
		return nil, errors.New("admin token expired")
	}
	if !admintoken.Equal(row.TokenHash, admintoken.Hash(raw, a.pepper)) {
		return nil, errors.New("invalid admin token")
	}
	_, _ = a.client.AdminAPIToken.UpdateOneID(row.ID).SetLastUsedAt(a.now()).Save(ctx)
	return row, nil
}

func (a adminAuthenticator) middleware(requiredScope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, "unauthorized", "A valid administrator token is required.")
			return
		}
		identity, err := a.authenticate(c.Request.Context(), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")))
		if err != nil {
			writeError(c, http.StatusUnauthorized, "unauthorized", "A valid administrator token is required.")
			return
		}
		if requiredScope != "" && !admintoken.HasScope(identity.Scopes, requiredScope) {
			writeError(c, http.StatusForbidden, "insufficient_scope", "The administrator token does not have the required scope.")
			return
		}
		ctx := context.WithValue(c.Request.Context(), adminIdentityKey{}, identity)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func requireScope(requiredScope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := adminIdentity(c)
		if identity == nil {
			writeError(c, http.StatusUnauthorized, "unauthorized", "A valid administrator token is required.")
			return
		}
		if !admintoken.HasScope(identity.Scopes, requiredScope) {
			writeError(c, http.StatusForbidden, "insufficient_scope", "The administrator token does not have the required scope.")
			return
		}
		c.Next()
	}
}

func adminIdentity(c *gin.Context) *ent.AdminAPIToken {
	identity, _ := c.Request.Context().Value(adminIdentityKey{}).(*ent.AdminAPIToken)
	return identity
}
