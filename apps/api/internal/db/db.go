package db

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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

func dbName(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}
