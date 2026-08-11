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

// EnsureInitialAdmin applies the versioned initial-admin migration once. It
// creates a missing account or repairs the pre-migration demo account, then
// records bootstrapVersion so later password changes survive every restart.
func EnsureInitialAdmin(ctx context.Context, database *mongo.Database, email, password string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return false, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, fmt.Errorf("hash initial admin password: %w", err)
	}
	users := database.Collection("users")
	var existing bson.M
	err = users.FindOne(ctx, bson.M{"email": email}).Decode(&existing)
	if err != nil && err != mongo.ErrNoDocuments {
		return false, fmt.Errorf("find initial admin: %w", err)
	}
	if err == nil {
		versioned := false
		switch version := existing["bootstrapVersion"].(type) {
		case int32:
			versioned = version >= 1
		case int64:
			versioned = version >= 1
		case int:
			versioned = version >= 1
		}
		if versioned {
			return false, nil
		}
		_, err = users.UpdateOne(ctx, bson.M{"_id": existing["_id"]}, bson.M{"$set": bson.M{
			"passwordHash": string(hash), "invitationStatus": "accepted",
			"bootstrapVersion": int32(1), "updatedAt": time.Now(),
		}})
		if err != nil {
			return false, fmt.Errorf("repair initial admin: %w", err)
		}
		return true, nil
	}
	now := time.Now()
	_, err = users.InsertOne(ctx, bson.M{
		"email": email, "name": "REMI Administrator", "role": "super-admin",
		"passwordHash": string(hash), "invitationStatus": "accepted",
		"bootstrapVersion": int32(1), "createdAt": now, "updatedAt": now,
	})
	if err != nil {
		return false, fmt.Errorf("create initial admin: %w", err)
	}
	_, _ = users.Indexes().CreateOne(ctx,
		mongo.IndexModel{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)})
	return true, nil
}

func dbName(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}
