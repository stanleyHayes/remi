package services

import "testing"

func TestAuthenticatedCloudinarySignatureIncludesPrivateDeliveryType(t *testing.T) {
	service := NewCloudinaryService("remi-cloud", "api-key", "api-secret")
	public, err := service.Signature("remi/leadership")
	if err != nil {
		t.Fatal(err)
	}
	private, err := service.AuthenticatedSignature("remi/finance/settlements")
	if err != nil {
		t.Fatal(err)
	}
	if private["type"] != "authenticated" || private["folder"] != "remi/finance/settlements" {
		t.Fatalf("private signature=%v", private)
	}
	if private["signature"] == public["signature"] {
		t.Fatal("authenticated delivery type was not covered by the signature")
	}
}
