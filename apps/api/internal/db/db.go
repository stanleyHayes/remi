package db

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

// Connect opens a MongoDB client and returns the database named in the URI path.
func Connect(ctx context.Context, uri string) (*mongo.Client, *mongo.Database, error) {
	name := dbName(uri)
	if name == "" {
		return nil, nil, fmt.Errorf("MONGODB_URI must include a database name, e.g. mongodb://host:27017/remi")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		return nil, nil, fmt.Errorf("ping mongo: %w", err)
	}
	return client, client.Database(name), nil
}

// EnsureInitialAdmin creates the explicitly configured deployment
// administrator when that email does not exist. SetOnInsert is intentional:
// later profile or password changes must survive service restarts.
func EnsureInitialAdmin(ctx context.Context, database *mongo.Database, email, password string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return false, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, fmt.Errorf("hash initial admin password: %w", err)
	}
	now := time.Now()
	result, err := database.Collection("users").UpdateOne(ctx,
		bson.M{"email": email},
		bson.M{"$setOnInsert": bson.M{
			"email": email, "name": "REMI Administrator", "role": "super-admin",
			"passwordHash": string(hash), "invitationStatus": "accepted",
			"createdAt": now, "updatedAt": now,
		}},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return false, fmt.Errorf("ensure initial admin: %w", err)
	}
	_, _ = database.Collection("users").Indexes().CreateOne(ctx,
		mongo.IndexModel{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)})
	return result.UpsertedCount == 1, nil
}

func dbName(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}
