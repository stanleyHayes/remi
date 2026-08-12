package db

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// RequireTransactions rejects standalone MongoDB in production. Finance,
// safeguarding and workflow commands rely on multi-document atomicity; the
// local standalone fallback is intentionally a development convenience only.
func RequireTransactions(ctx context.Context, database *mongo.Database) error {
	var hello bson.M
	if err := database.RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return fmt.Errorf("inspect MongoDB topology: %w", err)
	}
	if _, replica := hello["setName"]; replica {
		return nil
	}
	if message, _ := hello["msg"].(string); message == "isdbgrid" {
		return nil
	}
	return fmt.Errorf("production MongoDB must be a replica set or sharded cluster with transaction support")
}
