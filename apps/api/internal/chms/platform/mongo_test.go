package platform

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/testsupport"
)

func TestWithTransactionSupportsReplicaSetsAndStandaloneDevelopmentMongo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	database := client.Database("remi_platform_transaction_test")
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = database.Drop(cleanupCtx)
		_ = client.Disconnect(cleanupCtx)
	})
	store, _ := NewMongoPlatformStore(database)
	if err := store.WithTransaction(ctx, func(tx context.Context) error {
		_, insertErr := database.Collection("probe").InsertOne(tx, bson.M{"_id": "transaction-probe", "createdAt": time.Now().UTC()})
		return insertErr
	}); err != nil {
		t.Fatalf("transaction compatibility: %v", err)
	}
	count, err := database.Collection("probe").CountDocuments(ctx, bson.M{"_id": "transaction-probe"})
	if err != nil || count != 1 {
		t.Fatalf("transaction callback was not applied exactly once: count=%d err=%v", count, err)
	}
}

func TestStaleIdempotencyCommandCanBeSafelyReclaimed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	database := client.Database("remi_platform_idempotency_reclaim_test")
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = database.Drop(cleanup)
		_ = client.Disconnect(cleanup)
	})
	store, _ := NewMongoPlatformStore(database)
	if err = store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-11 * time.Minute)
	expiry := old.Add(24 * time.Hour)
	_, err = database.Collection(idempotencyCollection).InsertOne(ctx, IdempotencyRecord{Scope: "finance", Key: "command-0001", RequestHash: "hash", State: "processing", CreatedAt: old, ExpiresAt: &expiry})
	if err != nil {
		t.Fatal(err)
	}
	nextExpiry := time.Now().UTC().Add(24 * time.Hour)
	record, acquired, err := store.Begin(ctx, "finance", "command-0001", "hash", time.Now().UTC(), &nextExpiry)
	if err != nil || !acquired || !record.CreatedAt.After(old) {
		t.Fatalf("record=%+v acquired=%v err=%v", record, acquired, err)
	}
	if err = store.Complete(ctx, "finance", "command-0001", "hash", 201, []byte(`{"ok":true}`), "resource-1"); err != nil {
		t.Fatal(err)
	}
}
