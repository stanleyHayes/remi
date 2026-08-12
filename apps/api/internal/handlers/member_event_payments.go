package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"remi-api/internal/chms/finance"
	"remi-api/internal/chms/platform"
	"remi-api/internal/handlers/httpx"
)

const memberEventPaymentWindow = 20 * time.Minute

// StartMemberRegistrationPayment creates one checkout for every pending place
// in the member's booking. Event fees are intentionally kept separate from
// tax/charitable giving records.
func (h *Handler) StartMemberRegistrationPayment(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	now := time.Now().UTC()
	h.releaseExpiredMemberEventPayments(r.Context(), now)
	registrationID, err := bson.ObjectIDFromHex(strings.TrimSpace(chi.URLParam(r, "registrationId")))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "registration not found")
		return
	}
	var registration bson.M
	if h.DB.Collection("event_registrations").FindOne(r.Context(), bson.M{"_id": registrationID, "organizationId": h.Cfg.CHMSOrganizationID, "registeredByPersonId": claims.PersonID, "state": "pending-payment", "activeKey": true, "paymentExpiresAt": bson.M{"$gt": now}}).Decode(&registration) != nil {
		httpx.Error(w, http.StatusNotFound, "pending registration not found")
		return
	}
	bookingID := platform.ID(strings.TrimSpace(fmt.Sprint(registration["bookingId"])))
	if !bookingID.Valid() {
		httpx.Error(w, http.StatusConflict, "registration payment is unavailable")
		return
	}
	var event bson.M
	eventID, eventErr := bson.ObjectIDFromHex(strings.TrimSpace(fmt.Sprint(registration["eventId"])))
	if eventErr != nil || h.DB.Collection("events").FindOne(r.Context(), bson.M{"_id": eventID, "paymentRequired": true}, options.FindOne().SetProjection(bson.M{"title": 1, "priceMinor": 1, "currency": 1})).Decode(&event) != nil {
		httpx.Error(w, http.StatusConflict, "event payment is unavailable")
		return
	}
	count, err := h.DB.Collection("event_registrations").CountDocuments(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "bookingId": bookingID, "registeredByPersonId": claims.PersonID, "state": "pending-payment", "activeKey": true, "paymentExpiresAt": bson.M{"$gt": now}})
	priceMinor := int64(numericInt(event["priceMinor"]))
	currency := strings.ToUpper(strings.TrimSpace(fmt.Sprint(event["currency"])))
	if err != nil || count < 1 || priceMinor < 1 || currency == "" || currency == "<NIL>" {
		httpx.Error(w, http.StatusConflict, "event payment is unavailable")
		return
	}
	amountMinor := priceMinor * count
	collection := h.DB.Collection("chms_event_registration_payments")
	var existing bson.M
	existingErr := collection.FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "bookingId": bookingID, "registeredByPersonId": claims.PersonID}).Decode(&existing)
	if existingErr == nil && fmt.Sprint(existing["state"]) == "paid" {
		httpx.JSON(w, http.StatusOK, memberEventPaymentResponse(existing, false))
		return
	}
	if existingErr == nil && fmt.Sprint(existing["state"]) == "initialized" && strings.TrimSpace(fmt.Sprint(existing["authorizationUrl"])) != "" {
		httpx.JSON(w, http.StatusOK, memberEventPaymentResponse(existing, !h.Paystack.Configured()))
		return
	}
	paymentID := bson.NewObjectID()
	if existingErr == nil {
		if parsed, parseErr := bson.ObjectIDFromHex(fmt.Sprint(normalizeID(existing["_id"]))); parseErr == nil {
			paymentID = parsed
		}
	}
	reference := "REMI-EVENT-" + strings.ToUpper(paymentID.Hex())
	base := bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "bookingId": bookingID, "eventId": registration["eventId"], "registeredByPersonId": claims.PersonID, "registrationCount": count, "amountMinor": amountMinor, "currency": currency, "email": strings.ToLower(strings.TrimSpace(claims.Email)), "reference": reference, "state": "initializing", "expiresAt": registration["paymentExpiresAt"], "updatedAt": now}
	if existingErr == mongo.ErrNoDocuments {
		base["_id"], base["createdAt"] = paymentID, now
		if _, err = collection.InsertOne(r.Context(), base); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				httpx.Error(w, http.StatusConflict, "payment checkout is already being prepared")
				return
			}
			httpx.Error(w, http.StatusInternalServerError, "payment checkout is unavailable")
			return
		}
	} else if existingErr == nil {
		result, updateErr := collection.UpdateOne(r.Context(), bson.M{"_id": paymentID, "state": bson.M{"$in": bson.A{"failed", "initializing"}}}, bson.M{"$set": base})
		if updateErr != nil || result.ModifiedCount != 1 {
			httpx.Error(w, http.StatusConflict, "payment checkout is already being prepared")
			return
		}
	} else {
		httpx.Error(w, http.StatusInternalServerError, "payment checkout is unavailable")
		return
	}
	callbackURL := strings.TrimRight(h.Cfg.MemberAppURL, "/") + "/participation?registration_payment=" + paymentID.Hex()
	authorizationURL, demo := callbackURL+"&demo=1", !h.Paystack.Configured()
	if !demo {
		initialized, initializeErr := h.Paystack.InitializePayment(r.Context(), finance.ProviderInitializeRequest{AmountMinor: amountMinor, Currency: currency, Email: claims.Email, Reference: reference, CallbackURL: callbackURL, Metadata: map[string]any{"purpose": "event-registration", "payment_id": paymentID.Hex(), "booking_id": bookingID, "event_id": registration["eventId"], "organization_id": h.Cfg.CHMSOrganizationID}})
		if initializeErr != nil || initialized == nil || initialized.Reference != reference || strings.TrimSpace(initialized.AuthorizationURL) == "" {
			_, _ = collection.UpdateOne(r.Context(), bson.M{"_id": paymentID, "state": "initializing"}, bson.M{"$set": bson.M{"state": "failed", "failureCode": "provider-initialize", "updatedAt": time.Now().UTC()}})
			httpx.Error(w, http.StatusServiceUnavailable, "payment provider is temporarily unavailable")
			return
		}
		authorizationURL = initialized.AuthorizationURL
	}
	result, err := collection.UpdateOne(r.Context(), bson.M{"_id": paymentID, "state": "initializing", "reference": reference}, bson.M{"$set": bson.M{"state": "initialized", "authorizationUrl": authorizationURL, "demo": demo, "initializedAt": time.Now().UTC(), "updatedAt": time.Now().UTC()}})
	if err != nil || result.ModifiedCount != 1 {
		httpx.Error(w, http.StatusConflict, "payment checkout changed; try again")
		return
	}
	base["_id"], base["state"], base["authorizationUrl"], base["demo"] = paymentID, "initialized", authorizationURL, demo
	httpx.JSON(w, http.StatusCreated, memberEventPaymentResponse(base, demo))
}

func (h *Handler) ConfirmMemberRegistrationPayment(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	paymentID, err := bson.ObjectIDFromHex(strings.TrimSpace(chi.URLParam(r, "paymentId")))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "payment not found")
		return
	}
	var payment bson.M
	filter := bson.M{"_id": paymentID, "organizationId": h.Cfg.CHMSOrganizationID, "registeredByPersonId": claims.PersonID}
	if h.DB.Collection("chms_event_registration_payments").FindOne(r.Context(), filter).Decode(&payment) != nil {
		httpx.Error(w, http.StatusNotFound, "payment not found")
		return
	}
	if fmt.Sprint(payment["state"]) == "paid" {
		httpx.JSON(w, http.StatusOK, memberEventPaymentResponse(payment, payment["demo"] == true))
		return
	}
	now := time.Now().UTC()
	if expiry := timeValue(payment["expiresAt"]); expiry.IsZero() || !expiry.After(now) {
		h.releaseExpiredMemberEventPayments(r.Context(), now)
		httpx.Error(w, http.StatusGone, "payment window expired; the places were released")
		return
	}
	if fmt.Sprint(payment["state"]) != "initialized" {
		httpx.Error(w, http.StatusConflict, "payment is not ready for confirmation")
		return
	}
	reference := fmt.Sprint(payment["reference"])
	provider := bson.M{"reference": reference, "channel": "demo", "transactionId": "demo-" + paymentID.Hex(), "paidAt": now}
	if h.Paystack.Configured() {
		verified, verifyErr := h.Paystack.VerifyPayment(r.Context(), reference)
		if verifyErr != nil {
			httpx.Error(w, http.StatusServiceUnavailable, "payment verification is temporarily unavailable")
			return
		}
		if verified == nil || verified.Status != "success" || verified.Reference != reference || verified.AmountMinor != int64(numericInt(payment["amountMinor"])) || !strings.EqualFold(verified.Currency, fmt.Sprint(payment["currency"])) {
			httpx.Error(w, http.StatusConflict, "payment could not be matched to this registration")
			return
		}
		provider = bson.M{"reference": verified.Reference, "channel": verified.Channel, "transactionId": verified.TransactionID, "settlementReference": verified.SettlementReference, "paidAt": verified.PaidAt}
	} else if strings.EqualFold(strings.TrimSpace(h.Cfg.Environment), "production") || strings.EqualFold(strings.TrimSpace(h.Cfg.Environment), "prod") {
		httpx.Error(w, http.StatusServiceUnavailable, "payment provider is not configured")
		return
	}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	err = store.WithTransaction(r.Context(), func(tx context.Context) error {
		result, updateErr := h.DB.Collection("chms_event_registration_payments").UpdateOne(tx, bson.M{"_id": paymentID, "state": "initialized", "expiresAt": bson.M{"$gt": now}}, bson.M{"$set": bson.M{"state": "paid", "provider": provider, "paidAt": provider["paidAt"], "updatedAt": now}})
		if updateErr != nil {
			return updateErr
		}
		if result.ModifiedCount != 1 {
			return &platform.DomainError{Code: "conflict", Message: "Payment has already changed."}
		}
		registrations, updateErr := h.DB.Collection("event_registrations").UpdateMany(tx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "bookingId": payment["bookingId"], "registeredByPersonId": claims.PersonID, "state": "pending-payment", "activeKey": true, "paymentExpiresAt": bson.M{"$gt": now}}, bson.M{"$set": bson.M{"state": "confirmed", "paidAt": provider["paidAt"], "paymentId": paymentID.Hex(), "paymentReference": reference, "updatedAt": now}})
		if updateErr != nil {
			return updateErr
		}
		if registrations.ModifiedCount != int64(numericInt(payment["registrationCount"])) {
			return &platform.DomainError{Code: "conflict", Message: "Registration places changed before payment confirmation."}
		}
		return store.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}, Action: "member.registration.payment.confirm", ResourceType: "event-registration-payment", ResourceID: platform.ID(paymentID.Hex()), SubjectIDs: []platform.ID{platform.ID(claims.PersonID)}, ChangedFields: []string{"state", "provider", "paidAt"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		var winner bson.M
		if h.DB.Collection("chms_event_registration_payments").FindOne(r.Context(), bson.M{"_id": paymentID, "organizationId": h.Cfg.CHMSOrganizationID, "registeredByPersonId": claims.PersonID, "state": "paid"}).Decode(&winner) == nil {
			httpx.JSON(w, http.StatusOK, memberEventPaymentResponse(winner, winner["demo"] == true))
			return
		}
		platform.WriteError(w, r, err)
		return
	}
	payment["state"], payment["paidAt"], payment["provider"] = "paid", provider["paidAt"], provider
	httpx.JSON(w, http.StatusOK, memberEventPaymentResponse(payment, payment["demo"] == true))
}

func memberEventPaymentResponse(value bson.M, demo bool) bson.M {
	return bson.M{"id": normalizeID(value["_id"]), "bookingId": value["bookingId"], "state": value["state"], "amountMinor": value["amountMinor"], "currency": value["currency"], "authorizationUrl": value["authorizationUrl"], "expiresAt": value["expiresAt"], "paidAt": value["paidAt"], "demo": demo}
}

func (h *Handler) releaseExpiredMemberEventPayments(ctx context.Context, now time.Time) {
	cursor, err := h.DB.Collection("event_registrations").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "state": "pending-payment", "activeKey": true, "paymentExpiresAt": bson.M{"$lte": now}}, options.Find().SetProjection(bson.M{"bookingId": 1, "eventId": 1}).SetLimit(100))
	if err != nil {
		return
	}
	defer cursor.Close(ctx)
	var rows []bson.M
	if cursor.All(ctx, &rows) != nil {
		return
	}
	seen := map[string]bool{}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	for _, row := range rows {
		bookingID := strings.TrimSpace(fmt.Sprint(row["bookingId"]))
		if bookingID == "" || seen[bookingID] {
			continue
		}
		seen[bookingID] = true
		_ = store.WithTransaction(ctx, func(tx context.Context) error {
			result, updateErr := h.DB.Collection("event_registrations").UpdateMany(tx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "bookingId": bookingID, "state": "pending-payment", "activeKey": true, "paymentExpiresAt": bson.M{"$lte": now}}, bson.M{"$set": bson.M{"state": "cancelled", "activeKey": false, "cancelledAt": now, "cancellationReason": "payment-expired", "updatedAt": now}})
			if updateErr != nil || result.ModifiedCount == 0 {
				return updateErr
			}
			eventID, parseErr := bson.ObjectIDFromHex(strings.TrimSpace(fmt.Sprint(row["eventId"])))
			if parseErr != nil {
				return parseErr
			}
			var event bson.M
			if updateErr = h.DB.Collection("events").FindOne(tx, bson.M{"_id": eventID}, options.FindOne().SetProjection(bson.M{"paymentRequired": 1})).Decode(&event); updateErr != nil {
				return updateErr
			}
			promoted := int64(0)
			for promoted < result.ModifiedCount {
				promotion := bson.M{"state": "confirmed", "promotedAt": now, "updatedAt": now}
				if required, _ := event["paymentRequired"].(bool); required {
					promotion["state"] = "pending-payment"
					promotion["bookingId"] = platform.ID(bson.NewObjectID().Hex())
					promotion["paymentExpiresAt"] = now.Add(memberEventPaymentWindow)
				}
				promotionResult := h.DB.Collection("event_registrations").FindOneAndUpdate(tx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "eventId": row["eventId"], "state": "waitlisted"}, bson.M{"$set": promotion}, options.FindOneAndUpdate().SetSort(bson.D{{Key: "waitlistedAt", Value: 1}, {Key: "_id", Value: 1}}))
				if promotionResult.Err() == mongo.ErrNoDocuments {
					break
				}
				if promotionResult.Err() != nil {
					return promotionResult.Err()
				}
				promoted++
			}
			released := result.ModifiedCount - promoted
			if released > 0 {
				if _, updateErr = h.DB.Collection("events").UpdateOne(tx, bson.M{"_id": eventID, "registeredCount": bson.M{"$gte": released}}, bson.M{"$inc": bson.M{"registeredCount": -released}}); updateErr != nil {
					return updateErr
				}
			}
			_, updateErr = h.DB.Collection("chms_event_registration_payments").UpdateOne(tx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "bookingId": bookingID, "state": bson.M{"$ne": "paid"}}, bson.M{"$set": bson.M{"state": "expired", "expiredAt": now, "updatedAt": now}})
			return updateErr
		})
	}
}
