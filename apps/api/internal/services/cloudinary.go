package services

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// CloudinaryService produces client-side upload signatures.
type CloudinaryService struct {
	cloudName string
	apiKey    string
	apiSecret string
}

func NewCloudinaryService(cloudName, apiKey, apiSecret string) *CloudinaryService {
	return &CloudinaryService{cloudName: cloudName, apiKey: apiKey, apiSecret: apiSecret}
}

func (c *CloudinaryService) Configured() bool {
	return c.cloudName != "" && c.apiKey != "" && c.apiSecret != ""
}

// Signature returns the params a browser needs for a signed direct upload.
func (c *CloudinaryService) Signature(folder string) (map[string]any, error) {
	if !c.Configured() {
		return nil, errors.New("cloudinary not configured")
	}
	if folder == "" {
		folder = "remi"
	}
	timestamp := time.Now().Unix()

	params := map[string]string{
		"folder":    folder,
		"timestamp": fmt.Sprint(timestamp),
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "&") + c.apiSecret))

	return map[string]any{
		"cloudName": c.cloudName,
		"apiKey":    c.apiKey,
		"timestamp": timestamp,
		"folder":    folder,
		"signature": hex.EncodeToString(sum[:]),
	}, nil
}
