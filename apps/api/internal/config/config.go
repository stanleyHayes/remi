package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Environment        string
	Port               string
	MongoURI           string
	JWTSecret          string
	CORSOrigins        []string
	SeedAdminEmail     string
	SeedAdminPassword  string
	SeedMemberOTP      string
	CHMSOrganizationID string

	CloudinaryCloudName string
	CloudinaryAPIKey    string
	CloudinaryAPISecret string

	ResendAPIKey               string
	EmailFrom                  string
	NotifyEmail                string
	AdminAppURL                string
	MemberAppURL               string
	ArkeselAPIKey              string
	SMSSender                  string
	WhatsAppAccessToken        string
	WhatsAppPhoneNumberID      string
	WhatsAppGraphVersion       string
	PublicWebURL               string
	CommunicationWebhookSecret string

	PaystackSecretKey       string
	RetentionSignalsEnabled bool
}

// Load reads a .env file in the working directory (if present) without
// overriding variables already set in the environment, then applies defaults.
func Load() *Config {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if env == "production" || env == "prod" {
		loadDotEnv(".env.production")
	} else {
		loadDotEnv(".env")
	}

	cfg := &Config{
		Environment:                get("APP_ENV", get("ENV", "development")),
		Port:                       get("PORT", "8080"),
		MongoURI:                   get("MONGODB_URI", "mongodb://localhost:27019/remi"),
		JWTSecret:                  get("JWT_SECRET", "dev-secret-change-me"),
		SeedAdminEmail:             get("SEED_ADMIN_EMAIL", "admin@remi.church"),
		SeedAdminPassword:          get("SEED_ADMIN_PASSWORD", "remi-admin-2026"),
		SeedMemberOTP:              get("SEED_MEMBER_OTP", "260811"),
		CHMSOrganizationID:         get("CHMS_ORGANIZATION_ID", "remi"),
		CloudinaryCloudName:        get("CLOUDINARY_CLOUD_NAME", ""),
		CloudinaryAPIKey:           get("CLOUDINARY_API_KEY", ""),
		CloudinaryAPISecret:        get("CLOUDINARY_API_SECRET", ""),
		ResendAPIKey:               get("RESEND_API_KEY", ""),
		EmailFrom:                  get("EMAIL_FROM", "REMI Church <noreply@remi.church>"),
		NotifyEmail:                get("NOTIFY_EMAIL", "pastor@remi.church"),
		AdminAppURL:                strings.TrimRight(get("ADMIN_APP_URL", "http://localhost:3011"), "/"),
		MemberAppURL:               strings.TrimRight(get("MEMBER_APP_URL", "http://localhost:3012"), "/"),
		ArkeselAPIKey:              get("ARKESEL_API_KEY", ""),
		SMSSender:                  get("SMS_SENDER", "REMI"),
		WhatsAppAccessToken:        get("WHATSAPP_ACCESS_TOKEN", ""),
		WhatsAppPhoneNumberID:      get("WHATSAPP_PHONE_NUMBER_ID", ""),
		WhatsAppGraphVersion:       get("WHATSAPP_GRAPH_VERSION", "v23.0"),
		PublicWebURL:               strings.TrimRight(get("PUBLIC_WEB_URL", "http://localhost:3000"), "/"),
		CommunicationWebhookSecret: get("COMMUNICATION_WEBHOOK_SECRET", ""),
		PaystackSecretKey:          get("PAYSTACK_SECRET_KEY", ""),
		RetentionSignalsEnabled:    strings.EqualFold(get("CHMS_RETENTION_SIGNALS_ENABLED", "false"), "true"),
	}

	origins := get("CORS_ORIGINS", "http://localhost:3000,http://localhost:3001")
	for _, o := range strings.Split(origins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.CORSOrigins = append(cfg.CORSOrigins, o)
		}
	}
	return cfg
}

// ValidateProduction prevents known development defaults from reaching a live
// deployment. Development and test environments retain their convenient local
// defaults; production must supply explicit secrets and infrastructure.
func (c *Config) ValidateProduction() error {
	env := strings.ToLower(strings.TrimSpace(c.Environment))
	if env != "production" && env != "prod" {
		return nil
	}
	var missing []string
	if c.JWTSecret == "" || c.JWTSecret == "dev-secret-change-me" || len(c.JWTSecret) < 32 {
		missing = append(missing, "JWT_SECRET (at least 32 characters)")
	}
	if c.MongoURI == "" || strings.Contains(c.MongoURI, "localhost") {
		missing = append(missing, "MONGODB_URI (non-local production database)")
	}
	if len(c.CORSOrigins) == 0 {
		missing = append(missing, "CORS_ORIGINS")
	}
	if strings.TrimSpace(c.PaystackSecretKey) == "" {
		missing = append(missing, "PAYSTACK_SECRET_KEY")
	}
	if (strings.TrimSpace(c.ResendAPIKey) != "" || strings.TrimSpace(c.ArkeselAPIKey) != "" || strings.TrimSpace(c.WhatsAppAccessToken) != "") && strings.TrimSpace(c.CommunicationWebhookSecret) == "" {
		missing = append(missing, "COMMUNICATION_WEBHOOK_SECRET (required when a communication provider is configured)")
	}
	if len(missing) > 0 {
		return fmt.Errorf("invalid production configuration: %s", strings.Join(missing, ", "))
	}
	return nil
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}
