package revalidation

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	_ "github.com/mattn/go-sqlite3"

	"github.com/forecho/6url/backend/ent/enttest"
	"github.com/forecho/6url/backend/ent/revalidationjob"
)

func TestWorkerSignsAndCompletesRevalidation(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:revalidation-worker?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	row := client.RevalidationJob.Create().SetDedupeKey("publish:1").SetPaths([]string{"/", "/en/sites/example"}).SaveX(ctx)
	secret := "01234567890123456789012345678901"
	now := time.Unix(1_800_000_000, 0).UTC()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var payload webhookPayload
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.EventID != strconv.Itoa(row.ID) || payload.Timestamp != now.Unix() || len(payload.Paths) != 2 {
			t.Errorf("unexpected payload: %#v", payload)
		}
		body, _ := json.Marshal(payload)
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = fmt.Fprintf(mac, "%d.", payload.Timestamp)
		_, _ = mac.Write(body)
		expected := fmt.Sprintf("t=%d,v1=%s", payload.Timestamp, hex.EncodeToString(mac.Sum(nil)))
		if !hmac.Equal([]byte(request.Header.Get("X-6URL-Signature")), []byte(expected)) {
			t.Errorf("invalid signature: %s", request.Header.Get("X-6URL-Signature"))
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"revalidated":true}`))
	}))
	defer server.Close()
	worker := Worker{Client: client, Endpoint: server.URL, Secret: secret, Now: func() time.Time { return now }}
	if processed := worker.ProcessDue(ctx); processed != 1 {
		t.Fatalf("processed=%d", processed)
	}
	updated := client.RevalidationJob.GetX(ctx, row.ID)
	if updated.Status != revalidationjob.StatusCompleted || updated.CompletedAt == nil || strings.TrimSpace(updated.LastError) != "" {
		t.Fatalf("unexpected job: %#v", updated)
	}
}
