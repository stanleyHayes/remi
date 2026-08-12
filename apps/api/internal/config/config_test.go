package config

import (
	"strings"
	"testing"
)

func TestValidateProductionRequiresPaymentProvider(t *testing.T) {
	cfg := &Config{Environment: "production", MongoURI: "mongodb+srv://example.invalid/remi", JWTSecret: strings.Repeat("x", 32), CORSOrigins: []string{"https://example.invalid"}}
	if err := cfg.ValidateProduction(); err == nil || !strings.Contains(err.Error(), "PAYSTACK_SECRET_KEY") {
		t.Fatalf("missing Paystack key error=%v", err)
	}
	cfg.PaystackSecretKey = "sk_test_configured"
	if err := cfg.ValidateProduction(); err != nil {
		t.Fatalf("configured production: %v", err)
	}
}
