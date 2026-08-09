package database

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	_ "github.com/mattn/go-sqlite3"

	"github.com/forecho/6url/backend/ent/enttest"
)

func TestPaymentOrderRejectsMixedOrMissingBusinessSubject(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:order-constraints?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	_, err := client.PaymentOrder.Create().
		SetPurpose("listing").
		SetProvider("stripe").
		SetAmountMinor(1000).
		SetCurrency("usd").
		Save(context.Background())
	if err == nil {
		t.Fatal("listing order without a submission and listing plan was accepted")
	}
}

func TestApplicationSessionRequiresExactlyOneMatchingSubject(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:session-constraints?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	_, err := client.ApplicationSession.Create().
		SetSubjectType("submission").
		SetSessionHash("session-hash").
		SetCsrfSecretHash("csrf-hash").
		SetExpiresAt(time.Now().AddDate(0, 0, 1)).
		Save(context.Background())
	if err == nil {
		t.Fatal("application session without a submission was accepted")
	}
}
