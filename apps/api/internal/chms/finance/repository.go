package finance

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
)

const (
	fundsCollection              = "chms_finance_funds"
	paymentMethodsCollection     = "chms_finance_payment_methods"
	campusSettingsCollection     = "chms_finance_campuses"
	fiscalPeriodsCollection      = "chms_finance_periods"
	receiptSequencesCollection   = "chms_finance_receipt_sequences"
	accountMappingsCollection    = "chms_finance_account_mappings"
	periodLocksCollection        = "chms_finance_period_locks"
	contributionsCollection      = "chms_finance_contributions"
	receiptAllocationsCollection = "chms_finance_receipt_allocations"
	countingBatchesCollection    = "chms_finance_counting_batches"
	batchEntriesCollection       = "chms_finance_batch_entries"
	batchConfirmationsCollection = "chms_finance_batch_confirmations"
	batchEventsCollection        = "chms_finance_batch_events"
	paymentIntentsCollection     = "chms_finance_payment_intents"
	providerInboxCollection      = "chms_finance_provider_inbox"
	providerExceptionsCollection = "chms_finance_provider_exceptions"
	financeExportRunsCollection  = "chms_finance_export_runs"
)

type Repository struct{ database *mongo.Database }

func NewRepository(database *mongo.Database) (*Repository, error) {
	if database == nil {
		return nil, errors.New("finance database is required")
	}
	return &Repository{database: database}, nil
}

func (r *Repository) EnsureIndexes(ctx context.Context) error {
	definitions := map[string][]mongo.IndexModel{
		financeExportRunsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "artifactHash", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		statementRunsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "scopeKey", Value: 1}, {Key: "sourceHash", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "scopeKey", Value: 1}, {Key: "statementVersion", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "subject.type", Value: 1}, {Key: "subject.id", Value: 1}, {Key: "generatedAt", Value: -1}}},
		},
		documentDeliveriesCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "documentType", Value: 1}, {Key: "documentId", Value: 1}, {Key: "requestKey", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "state", Value: 1}, {Key: "requestedAt", Value: 1}}},
		},
		settlementsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "sourceType", Value: 1}, {Key: "fileHash", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "periodId", Value: 1}, {Key: "state", Value: 1}, {Key: "settledAt", Value: -1}}},
		},
		reconciliationItemsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "settlementId", Value: 1}, {Key: "sourceRowId", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "resolution", Value: 1}, {Key: "ownerId", Value: 1}, {Key: "createdAt", Value: 1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "matchedTargetType", Value: 1}, {Key: "matchedTargetId", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"matchedTargetId": bson.M{"$type": "string"}})},
		},
		periodControlRequestsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "periodId", Value: 1}, {Key: "action", Value: 1}, {Key: "state", Value: 1}}},
		},
		periodControlEventsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "periodId", Value: 1}, {Key: "occurredAt", Value: 1}}},
		},
		pledgesCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "donor.personId", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "donor.householdId", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "campaignId", Value: 1}, {Key: "state", Value: 1}}},
		},
		pledgeRemindersCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "pledgeId", Value: 1}, {Key: "channel", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "requestId", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		campaignsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "slug", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "status", Value: 1}, {Key: "featured", Value: -1}, {Key: "startsAt", Value: -1}}},
		},
		fundsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "code", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "activeFrom", Value: 1}, {Key: "activeUntil", Value: 1}}},
		},
		paymentMethodsCollection: {{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "code", Value: 1}}, Options: options.Index().SetUnique(true)}},
		campusSettingsCollection: {{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}}, Options: options.Index().SetUnique(true)}},
		fiscalPeriodsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "code", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "startsAt", Value: 1}, {Key: "endsAt", Value: 1}}},
		},
		receiptSequencesCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "code", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "fiscalYear", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		accountMappingsCollection: {{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "fundId", Value: 1}, {Key: "category", Value: 1}, {Key: "effectiveFrom", Value: 1}}, Options: options.Index().SetUnique(true)}},
		contributionsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "commandKey", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "receiptNumber", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "providerReference", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"providerReference": bson.M{"$type": "string"}})},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "sourceReceiptNumber", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"sourceReceiptNumber": bson.M{"$type": "string"}})},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "adjustment.reversesContributionId", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"adjustment.reversesContributionId": bson.M{"$type": "string"}})},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "adjustment.providerEventReference", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"adjustment.providerEventReference": bson.M{"$type": "string"}})},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "adjustment.providerAdjustsContributionId", Value: 1}, {Key: "postedAt", Value: 1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "receivedAt", Value: -1}, {Key: "_id", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "donor.personId", Value: 1}, {Key: "receivedAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "donor.householdId", Value: 1}, {Key: "receivedAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "campaignId", Value: 1}, {Key: "postedAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "pledgeId", Value: 1}, {Key: "postedAt", Value: -1}}},
		},
		receiptAllocationsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "receiptNumber", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "contributionId", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		memberPaymentMethodsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "state", Value: 1}, {Key: "updatedAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "provider", Value: 1}, {Key: "signature", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		recurringInstructionsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "state", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "state", Value: 1}, {Key: "nextChargeAt", Value: 1}, {Key: "leaseUntil", Value: 1}}},
		},
		countingBatchesCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "state", Value: 1}, {Key: "receivedAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "counterIds", Value: 1}, {Key: "state", Value: 1}}},
		},
		batchEntriesCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "batchId", Value: 1}, {Key: "archivedAt", Value: 1}, {Key: "createdAt", Value: 1}}},
		},
		batchConfirmationsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "batchId", Value: 1}, {Key: "counterId", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		batchEventsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "batchId", Value: 1}, {Key: "sequence", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		paymentIntentsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "provider", Value: 1}, {Key: "reference", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "clientRequestKey", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "state", Value: 1}, {Key: "updatedAt", Value: 1}}},
		},
		providerInboxCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "provider", Value: 1}, {Key: "bodyHash", Value: 1}, {Key: "signatureValid", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "state", Value: 1}, {Key: "nextAttemptAt", Value: 1}, {Key: "receivedAt", Value: 1}}},
		},
		providerExceptionsCollection: {
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "provider", Value: 1}, {Key: "inboxId", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "state", Value: 1}, {Key: "createdAt", Value: 1}}},
		},
	}
	for collection, indexes := range definitions {
		if _, err := r.database.Collection(collection).Indexes().CreateMany(ctx, indexes); err != nil {
			return fmt.Errorf("create %s indexes: %w", collection, err)
		}
	}
	return nil
}

func (r *Repository) Insert(ctx context.Context, collection string, value any) error {
	_, err := r.database.Collection(collection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "conflict", Message: "A finance record with that code or scope already exists."}
	}
	if err != nil {
		return fmt.Errorf("insert finance record: %w", err)
	}
	return nil
}
func (r *Repository) Update(ctx context.Context, collection string, org, id platform.ID, expected int64, set bson.M) error {
	result, err := r.database.Collection(collection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "version": expected, "archivedAt": nil}, bson.M{"$set": set})
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "conflict", Message: "A finance record with that code or scope already exists."}
	}
	if err != nil {
		return fmt.Errorf("update finance record: %w", err)
	}
	if result.MatchedCount == 0 {
		return platform.VersionConflict(expected)
	}
	return nil
}
func findOne[T any](ctx context.Context, r *Repository, collection string, org, id platform.ID) (*T, error) {
	var value T
	err := r.database.Collection(collection).FindOne(ctx, bson.M{"_id": id, "organizationId": org, "archivedAt": nil}).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find finance record: %w", err)
	}
	return &value, nil
}
func listAll[T any](ctx context.Context, r *Repository, collection string, org platform.ID) ([]T, error) {
	cursor, err := r.database.Collection(collection).Find(ctx, bson.M{"organizationId": org, "archivedAt": nil}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list finance records: %w", err)
	}
	defer cursor.Close(ctx)
	values := []T{}
	if err = cursor.All(ctx, &values); err != nil {
		return nil, fmt.Errorf("decode finance records: %w", err)
	}
	return values, nil
}
func (r *Repository) FindFund(ctx context.Context, org, id platform.ID) (*Fund, error) {
	return findOne[Fund](ctx, r, fundsCollection, org, id)
}
func (r *Repository) FindPaymentMethod(ctx context.Context, org, id platform.ID) (*PaymentMethod, error) {
	return findOne[PaymentMethod](ctx, r, paymentMethodsCollection, org, id)
}
func (r *Repository) FindCampusSettings(ctx context.Context, org, id platform.ID) (*CampusSettings, error) {
	return findOne[CampusSettings](ctx, r, campusSettingsCollection, org, id)
}
func (r *Repository) FindFiscalPeriod(ctx context.Context, org, id platform.ID) (*FiscalPeriod, error) {
	return findOne[FiscalPeriod](ctx, r, fiscalPeriodsCollection, org, id)
}
func (r *Repository) FindReceiptSequence(ctx context.Context, org, id platform.ID) (*ReceiptSequence, error) {
	return findOne[ReceiptSequence](ctx, r, receiptSequencesCollection, org, id)
}
func (r *Repository) FindAccountMapping(ctx context.Context, org, id platform.ID) (*AccountMapping, error) {
	return findOne[AccountMapping](ctx, r, accountMappingsCollection, org, id)
}
func (r *Repository) ListFunds(ctx context.Context, org platform.ID) ([]Fund, error) {
	return listAll[Fund](ctx, r, fundsCollection, org)
}
func (r *Repository) ListPaymentMethods(ctx context.Context, org platform.ID) ([]PaymentMethod, error) {
	return listAll[PaymentMethod](ctx, r, paymentMethodsCollection, org)
}
func (r *Repository) ListCampusSettings(ctx context.Context, org platform.ID) ([]CampusSettings, error) {
	return listAll[CampusSettings](ctx, r, campusSettingsCollection, org)
}
func (r *Repository) ListFiscalPeriods(ctx context.Context, org platform.ID) ([]FiscalPeriod, error) {
	return listAll[FiscalPeriod](ctx, r, fiscalPeriodsCollection, org)
}
func (r *Repository) ListReceiptSequences(ctx context.Context, org platform.ID) ([]ReceiptSequence, error) {
	return listAll[ReceiptSequence](ctx, r, receiptSequencesCollection, org)
}
func (r *Repository) ListAccountMappings(ctx context.Context, org platform.ID) ([]AccountMapping, error) {
	return listAll[AccountMapping](ctx, r, accountMappingsCollection, org)
}

func (r *Repository) HasOverlappingPeriod(ctx context.Context, org, exclude platform.ID, start, end time.Time) (bool, error) {
	filter := bson.M{"organizationId": org, "archivedAt": nil, "startsAt": bson.M{"$lt": end}, "endsAt": bson.M{"$gt": start}}
	if exclude.Valid() {
		filter["_id"] = bson.M{"$ne": exclude}
	}
	count, err := r.database.Collection(fiscalPeriodsCollection).CountDocuments(ctx, filter)
	return count > 0, err
}
func (r *Repository) LockPeriodSchedule(ctx context.Context, org platform.ID, now time.Time) error {
	_, err := r.database.Collection(periodLocksCollection).UpdateOne(ctx, bson.M{"_id": org}, bson.M{"$set": bson.M{"touchedAt": now}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("lock fiscal period schedule: %w", err)
	}
	return nil
}
func (r *Repository) FundExists(ctx context.Context, org, id platform.ID) (bool, error) {
	if !id.Valid() {
		return true, nil
	}
	count, err := r.database.Collection(fundsCollection).CountDocuments(ctx, bson.M{"_id": id, "organizationId": org, "archivedAt": nil})
	return count == 1, err
}

func (r *Repository) ReserveReceipt(ctx context.Context, org, sequenceID platform.ID, now time.Time, actor platform.Actor) (string, error) {
	var before ReceiptSequence
	err := r.database.Collection(receiptSequencesCollection).FindOneAndUpdate(ctx,
		bson.M{"_id": sequenceID, "organizationId": org, "archivedAt": nil},
		bson.M{"$inc": bson.M{"nextNumber": 1, "version": 1}, "$set": bson.M{"updatedAt": now, "updatedBy": actor}},
		options.FindOneAndUpdate().SetReturnDocument(options.Before),
	).Decode(&before)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return "", &platform.DomainError{Code: "not_found", Message: "Receipt sequence not found."}
	}
	if err != nil {
		return "", fmt.Errorf("reserve receipt number: %w", err)
	}
	number := strconv.FormatInt(before.NextNumber, 10)
	if len(number) < before.Padding {
		number = strings.Repeat("0", before.Padding-len(number)) + number
	}
	return fmt.Sprintf("%s-%d-%s", before.Prefix, before.FiscalYear, number), nil
}

func (r *Repository) FindPeriodForDate(ctx context.Context, org platform.ID, at time.Time) (*FiscalPeriod, error) {
	var value FiscalPeriod
	err := r.database.Collection(fiscalPeriodsCollection).FindOne(ctx, bson.M{"organizationId": org, "status": "open", "archivedAt": nil, "startsAt": bson.M{"$lte": at}, "endsAt": bson.M{"$gt": at}}).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find fiscal period: %w", err)
	}
	return &value, nil
}
func (r *Repository) FindSequenceForYear(ctx context.Context, org platform.ID, year int) (*ReceiptSequence, error) {
	var value ReceiptSequence
	err := r.database.Collection(receiptSequencesCollection).FindOne(ctx, bson.M{"organizationId": org, "fiscalYear": year, "archivedAt": nil}).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find receipt sequence: %w", err)
	}
	return &value, nil
}
func (r *Repository) FindActiveFund(ctx context.Context, org, id platform.ID, at time.Time) (*Fund, error) {
	var value Fund
	err := r.database.Collection(fundsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org, "archivedAt": nil, "activeFrom": bson.M{"$lte": at}, "$or": bson.A{bson.M{"activeUntil": nil}, bson.M{"activeUntil": bson.M{"$gt": at}}}}).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find active fund: %w", err)
	}
	return &value, nil
}
func (r *Repository) FindActivePaymentMethod(ctx context.Context, org, id platform.ID) (*PaymentMethod, error) {
	var value PaymentMethod
	err := r.database.Collection(paymentMethodsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org, "archivedAt": nil, "active": true}).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find active payment method: %w", err)
	}
	return &value, nil
}
func (r *Repository) CampusExists(ctx context.Context, org, branch platform.ID) (bool, error) {
	count, err := r.database.Collection(campusSettingsCollection).CountDocuments(ctx, bson.M{"organizationId": org, "branchId": branch, "archivedAt": nil})
	return count == 1, err
}
func (r *Repository) DonorExists(ctx context.Context, org platform.ID, donor DonorAttribution) (bool, error) {
	collection, id := "", platform.ID("")
	switch donor.Type {
	case "person":
		collection, id = "chms_people", donor.PersonID
	case "household":
		collection, id = "chms_households", donor.HouseholdID
	default:
		return true, nil
	}
	count, err := r.database.Collection(collection).CountDocuments(ctx, bson.M{"_id": id, "organizationId": org, "archivedAt": nil})
	return count == 1, err
}
func (r *Repository) InsertContribution(ctx context.Context, value Contribution) error {
	_, err := r.database.Collection(contributionsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert contribution: %w", err)
	}
	return nil
}
func (r *Repository) InsertReceiptAllocation(ctx context.Context, value ReceiptAllocation) error {
	_, err := r.database.Collection(receiptAllocationsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert receipt allocation: %w", err)
	}
	return nil
}
func (r *Repository) FindContribution(ctx context.Context, org, id platform.ID) (*Contribution, error) {
	return findOne[Contribution](ctx, r, contributionsCollection, org, id)
}
func (r *Repository) FindContributionByCommand(ctx context.Context, org platform.ID, key string) (*Contribution, error) {
	var value Contribution
	err := r.database.Collection(contributionsCollection).FindOne(ctx, bson.M{"organizationId": org, "commandKey": key}).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find contribution command: %w", err)
	}
	return &value, nil
}
func (r *Repository) FindReversal(ctx context.Context, org, originalID platform.ID) (*Contribution, error) {
	var value Contribution
	err := r.database.Collection(contributionsCollection).FindOne(ctx, bson.M{"organizationId": org, "adjustment.reversesContributionId": originalID}).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find contribution reversal: %w", err)
	}
	return &value, nil
}

func (r *Repository) SumProviderAdjustments(ctx context.Context, org, originalID platform.ID) (int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"organizationId": org, "adjustment.providerAdjustsContributionId": originalID}}},
		{{Key: "$group", Value: bson.M{"_id": nil, "total": bson.M{"$sum": "$total.amountMinor"}}}},
	}
	cursor, err := r.database.Collection(contributionsCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return 0, fmt.Errorf("sum provider adjustments: %w", err)
	}
	defer cursor.Close(ctx)
	var rows []struct {
		Total int64 `bson:"total"`
	}
	if err = cursor.All(ctx, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].Total, nil
}
func (r *Repository) FindReversals(ctx context.Context, org platform.ID, originalIDs []platform.ID) (map[platform.ID]Contribution, error) {
	result := map[platform.ID]Contribution{}
	if len(originalIDs) == 0 {
		return result, nil
	}
	cursor, err := r.database.Collection(contributionsCollection).Find(ctx, bson.M{"organizationId": org, "adjustment.reversesContributionId": bson.M{"$in": originalIDs}})
	if err != nil {
		return nil, fmt.Errorf("find contribution reversals: %w", err)
	}
	defer cursor.Close(ctx)
	var values []Contribution
	if err = cursor.All(ctx, &values); err != nil {
		return nil, fmt.Errorf("decode contribution reversals: %w", err)
	}
	for _, value := range values {
		if value.Link != nil {
			result[value.Link.ReversesContributionID] = value
		}
	}
	return result, nil
}
func (r *Repository) ListContributions(ctx context.Context, org, branch platform.ID, limit int64) ([]Contribution, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	filter := bson.M{"organizationId": org, "archivedAt": nil}
	if branch.Valid() {
		filter["branchId"] = branch
	}
	cursor, err := r.database.Collection(contributionsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "receivedAt", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list contributions: %w", err)
	}
	defer cursor.Close(ctx)
	values := []Contribution{}
	if err = cursor.All(ctx, &values); err != nil {
		return nil, fmt.Errorf("decode contributions: %w", err)
	}
	return values, nil
}

func (r *Repository) InsertPaymentIntent(ctx context.Context, value PaymentIntent) error {
	_, err := r.database.Collection(paymentIntentsCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "conflict", Message: "This payment checkout already exists."}
	}
	if err != nil {
		return fmt.Errorf("insert payment intent: %w", err)
	}
	return nil
}

func (r *Repository) FindPaymentIntentByClientKey(ctx context.Context, org platform.ID, key string) (*PaymentIntent, error) {
	var value PaymentIntent
	err := r.database.Collection(paymentIntentsCollection).FindOne(ctx, bson.M{"organizationId": org, "clientRequestKey": key}).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find payment intent key: %w", err)
	}
	return &value, nil
}

func (r *Repository) FindPaymentIntentByReference(ctx context.Context, org platform.ID, reference string) (*PaymentIntent, error) {
	var value PaymentIntent
	err := r.database.Collection(paymentIntentsCollection).FindOne(ctx, bson.M{"organizationId": org, "provider": "paystack", "reference": reference}).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find payment intent reference: %w", err)
	}
	return &value, nil
}

func (r *Repository) FindPaymentIntentByID(ctx context.Context, org, id platform.ID) (*PaymentIntent, error) {
	var value PaymentIntent
	err := r.database.Collection(paymentIntentsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org}).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Payment intent not found."}
	}
	if err != nil {
		return nil, fmt.Errorf("find payment intent: %w", err)
	}
	return &value, nil
}

func (r *Repository) UpdatePaymentIntent(ctx context.Context, org, id platform.ID, expected int64, set bson.M) error {
	result, err := r.database.Collection(paymentIntentsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "version": expected}, bson.M{"$set": set})
	if err != nil {
		return fmt.Errorf("update payment intent: %w", err)
	}
	if result.MatchedCount == 0 {
		return platform.VersionConflict(expected)
	}
	return nil
}

func (r *Repository) AdvancePaymentIntent(ctx context.Context, org, id platform.ID, set bson.M) error {
	set["updatedAt"] = time.Now().UTC()
	result, err := r.database.Collection(paymentIntentsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org}, bson.M{"$set": set, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("advance payment intent: %w", err)
	}
	if result.MatchedCount == 0 {
		return &platform.DomainError{Code: "not_found", Message: "Payment intent not found."}
	}
	return nil
}

func (r *Repository) AdvancePaymentIntentRefundState(ctx context.Context, org, id platform.ID, state string, now time.Time) error {
	allowed := map[string][]string{
		"pending":         {"pending"},
		"processing":      {"pending", "processing"},
		"needs-attention": {"pending", "processing", "needs-attention"},
		"failed":          {"pending", "processing", "needs-attention", "failed"},
	}
	states, ok := allowed[state]
	if !ok {
		return nil
	}
	result, err := r.database.Collection(paymentIntentsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "$or": bson.A{bson.M{"refundState": bson.M{"$exists": false}}, bson.M{"refundState": bson.M{"$in": states}}}}, bson.M{"$set": bson.M{"refundState": state, "lastProviderEventAt": now, "updatedAt": now}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("advance payment refund state: %w", err)
	}
	if result.MatchedCount == 0 {
		// A later terminal state already won; reordered older events are no-ops.
		return nil
	}
	return nil
}

func (r *Repository) ApplyPaymentIntentRefund(ctx context.Context, org, id platform.ID, eventReference string, amount int64, now time.Time) error {
	var current PaymentIntent
	err := r.database.Collection(paymentIntentsCollection).FindOneAndUpdate(ctx,
		bson.M{"_id": id, "organizationId": org, "appliedProviderEvents": bson.M{"$ne": eventReference}, "$expr": bson.M{"$lte": bson.A{bson.M{"$add": bson.A{bson.M{"$ifNull": bson.A{"$refundedAmountMinor", int64(0)}}, amount}}, "$total.amountMinor"}}},
		bson.M{"$set": bson.M{"refundState": "processed", "lastProviderEventAt": now, "updatedAt": now}, "$inc": bson.M{"refundedAmountMinor": amount, "version": 1}, "$addToSet": bson.M{"appliedProviderEvents": eventReference}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&current)
	if errors.Is(err, mongo.ErrNoDocuments) {
		stored, findErr := r.FindPaymentIntentByID(ctx, org, id)
		if findErr != nil {
			return findErr
		}
		for _, applied := range stored.AppliedProviderEvents {
			if applied == eventReference {
				return r.reconcilePaymentIntentRefundState(ctx, org, stored, now)
			}
		}
		return platform.ValidationError(platform.FieldError{Path: "amount", Code: "exceeds_remaining", Message: "Processed refunds exceed the original payment amount."})
	}
	if err != nil {
		return fmt.Errorf("apply payment refund: %w", err)
	}
	err = r.reconcilePaymentIntentRefundState(ctx, org, &current, now)
	return err
}

func (r *Repository) reconcilePaymentIntentRefundState(ctx context.Context, org platform.ID, current *PaymentIntent, now time.Time) error {
	state := "partially-refunded"
	if current.RefundedAmountMinor == current.Total.AmountMinor {
		state = "refunded"
	}
	_, err := r.database.Collection(paymentIntentsCollection).UpdateOne(ctx, bson.M{"_id": current.ID, "organizationId": org}, bson.M{"$set": bson.M{"state": state, "refundState": "processed", "updatedAt": now}})
	return err
}

func (r *Repository) ResolvePaymentBranch(ctx context.Context, org, requested platform.ID) (platform.ID, error) {
	if requested.Valid() {
		var value CampusSettings
		err := r.database.Collection(campusSettingsCollection).FindOne(ctx, bson.M{"organizationId": org, "branchId": requested, "archivedAt": nil}).Decode(&value)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", nil
		}
		return value.BranchID, err
	}
	cursor, err := r.database.Collection(campusSettingsCollection).Find(ctx, bson.M{"organizationId": org, "archivedAt": nil}, options.Find().SetLimit(2))
	if err != nil {
		return "", err
	}
	defer cursor.Close(ctx)
	var values []CampusSettings
	if err = cursor.All(ctx, &values); err != nil {
		return "", err
	}
	if len(values) != 1 {
		return "", nil
	}
	return values[0].BranchID, nil
}

func (r *Repository) ResolvePublicFund(ctx context.Context, org, requested platform.ID, category string, at time.Time) (*Fund, error) {
	filter := bson.M{"organizationId": org, "archivedAt": nil, "activeFrom": bson.M{"$lte": at}, "$or": bson.A{bson.M{"activeUntil": nil}, bson.M{"activeUntil": bson.M{"$gt": at}}}}
	if requested.Valid() {
		filter["_id"] = requested
	} else {
		pattern := "^" + regexp.QuoteMeta(strings.TrimSpace(category)) + "$"
		filter["$and"] = bson.A{bson.M{"$or": bson.A{bson.M{"code": bson.Regex{Pattern: pattern, Options: "i"}}, bson.M{"name": bson.Regex{Pattern: pattern, Options: "i"}}}}}
	}
	var value Fund
	err := r.database.Collection(fundsCollection).FindOne(ctx, filter).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve public fund: %w", err)
	}
	return &value, nil
}

func (r *Repository) ResolvePaystackMethod(ctx context.Context, org, requested platform.ID) (*PaymentMethod, error) {
	filter := bson.M{"organizationId": org, "archivedAt": nil, "active": true, "provider": bson.Regex{Pattern: "^paystack$", Options: "i"}, "kind": bson.M{"$in": bson.A{"card", "mobile-money", "bank-transfer"}}}
	if requested.Valid() {
		filter["_id"] = requested
	}
	var value PaymentMethod
	err := r.database.Collection(paymentMethodsCollection).FindOne(ctx, filter, options.FindOne().SetSort(bson.D{{Key: "code", Value: 1}})).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve Paystack payment method: %w", err)
	}
	return &value, nil
}

func (r *Repository) InsertProviderInbox(ctx context.Context, value ProviderInbox) (*ProviderInbox, bool, error) {
	_, err := r.database.Collection(providerInboxCollection).InsertOne(ctx, value)
	if err == nil {
		return &value, true, nil
	}
	if !mongo.IsDuplicateKeyError(err) {
		return nil, false, fmt.Errorf("insert provider inbox: %w", err)
	}
	var existing ProviderInbox
	if err = r.database.Collection(providerInboxCollection).FindOne(ctx, bson.M{"organizationId": value.OrganizationID, "provider": value.Provider, "bodyHash": value.BodyHash, "signatureValid": value.SignatureValid}).Decode(&existing); err != nil {
		return nil, false, fmt.Errorf("find duplicate provider inbox: %w", err)
	}
	return &existing, false, nil
}

func (r *Repository) ClaimProviderInbox(ctx context.Context, id platform.ID, now time.Time) (bool, error) {
	stale := now.Add(-5 * time.Minute)
	result, err := r.database.Collection(providerInboxCollection).UpdateOne(ctx, bson.M{"_id": id, "$or": bson.A{bson.M{"state": bson.M{"$in": bson.A{"received", "retry"}}}, bson.M{"state": "processing", "claimedAt": bson.M{"$lt": stale}}}}, bson.M{"$set": bson.M{"state": "processing", "claimedAt": now}, "$inc": bson.M{"attempts": 1}})
	if err != nil {
		return false, fmt.Errorf("claim provider inbox: %w", err)
	}
	return result.ModifiedCount == 1, nil
}

func (r *Repository) CompleteProviderInbox(ctx context.Context, id platform.ID, state, code, eventType, reference string, now time.Time) error {
	_, err := r.database.Collection(providerInboxCollection).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"state": state, "lastErrorCode": code, "eventType": eventType, "reference": reference, "processedAt": now}, "$unset": bson.M{"nextAttemptAt": ""}})
	return err
}

func (r *Repository) RetryProviderInbox(ctx context.Context, id platform.ID, code string, next time.Time) error {
	_, err := r.database.Collection(providerInboxCollection).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"state": "retry", "lastErrorCode": code, "nextAttemptAt": next}})
	return err
}

func (r *Repository) InsertProviderException(ctx context.Context, value ProviderException) error {
	_, err := r.database.Collection(providerExceptionsCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return nil
	}
	return err
}

func (r *Repository) ListPaymentIntents(ctx context.Context, org, branch platform.ID, limit int64) ([]PaymentIntent, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	cursor, err := r.database.Collection(paymentIntentsCollection).Find(ctx, bson.M{"organizationId": org, "branchId": branch}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []PaymentIntent{}
	err = cursor.All(ctx, &values)
	return values, err
}

func (r *Repository) ListProviderExceptions(ctx context.Context, org platform.ID, limit int64) ([]ProviderException, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	cursor, err := r.database.Collection(providerExceptionsCollection).Find(ctx, bson.M{"organizationId": org}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []ProviderException{}
	err = cursor.All(ctx, &values)
	return values, err
}

func (r *Repository) InsertBatch(ctx context.Context, value CountingBatch) error {
	return r.Insert(ctx, countingBatchesCollection, value)
}
func (r *Repository) FindBatch(ctx context.Context, org, id platform.ID) (*CountingBatch, error) {
	return findOne[CountingBatch](ctx, r, countingBatchesCollection, org, id)
}
func (r *Repository) ListBatches(ctx context.Context, org, branch, counterID platform.ID, limit int64) ([]CountingBatch, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	filter := bson.M{"organizationId": org, "branchId": branch, "archivedAt": nil}
	if counterID.Valid() {
		filter["counterIds"] = counterID
	}
	cursor, err := r.database.Collection(countingBatchesCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "receivedAt", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list counting batches: %w", err)
	}
	defer cursor.Close(ctx)
	values := []CountingBatch{}
	if err = cursor.All(ctx, &values); err != nil {
		return nil, fmt.Errorf("decode counting batches: %w", err)
	}
	return values, nil
}
func (r *Repository) StaffHasRole(ctx context.Context, id platform.ID, roles ...string) (bool, error) {
	var key any = id
	if parsed, err := bson.ObjectIDFromHex(string(id)); err == nil {
		key = parsed
	}
	count, err := r.database.Collection("users").CountDocuments(ctx, bson.M{"_id": key, "role": bson.M{"$in": roles}, "$or": bson.A{bson.M{"invitationStatus": "accepted"}, bson.M{"passwordHash": bson.M{"$exists": true, "$ne": ""}}}})
	return count == 1, err
}

func (r *Repository) ListActiveFinanceStaff(ctx context.Context) ([]ReconciliationOwner, error) {
	roles := bson.A{"super-admin", "finance-counter", "finance-admin", "finance-approver", "finance-auditor"}
	filter := bson.M{"role": bson.M{"$in": roles}, "$or": bson.A{bson.M{"invitationStatus": "accepted"}, bson.M{"passwordHash": bson.M{"$exists": true, "$ne": ""}}}}
	cursor, err := r.database.Collection("users").Find(ctx, filter, options.Find().SetProjection(bson.M{"name": 1, "email": 1, "role": 1}).SetSort(bson.D{{Key: "name", Value: 1}, {Key: "email", Value: 1}}).SetLimit(200))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	owners := []ReconciliationOwner{}
	for cursor.Next(ctx) {
		var value bson.M
		if err = cursor.Decode(&value); err != nil {
			return nil, err
		}
		var id platform.ID
		switch raw := value["_id"].(type) {
		case bson.ObjectID:
			id = platform.ID(raw.Hex())
		case string:
			id = platform.ID(raw)
		}
		if !id.Valid() {
			continue
		}
		owners = append(owners, ReconciliationOwner{ID: id, Name: fmt.Sprint(value["name"]), Email: fmt.Sprint(value["email"]), Role: fmt.Sprint(value["role"])})
	}
	return owners, cursor.Err()
}
func (r *Repository) UpdateBatch(ctx context.Context, org, id platform.ID, expected int64, states []string, set bson.M) error {
	filter := bson.M{"_id": id, "organizationId": org, "version": expected, "archivedAt": nil}
	if len(states) > 0 {
		filter["state"] = bson.M{"$in": states}
	}
	result, err := r.database.Collection(countingBatchesCollection).UpdateOne(ctx, filter, bson.M{"$set": set})
	if err != nil {
		return fmt.Errorf("update counting batch: %w", err)
	}
	if result.MatchedCount == 0 {
		return platform.VersionConflict(expected)
	}
	return nil
}
func (r *Repository) InsertBatchEntry(ctx context.Context, value BatchEntry) error {
	return r.Insert(ctx, batchEntriesCollection, value)
}
func (r *Repository) FindBatchEntry(ctx context.Context, org, batchID, id platform.ID) (*BatchEntry, error) {
	var value BatchEntry
	err := r.database.Collection(batchEntriesCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org, "batchId": batchID, "archivedAt": nil}).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find batch entry: %w", err)
	}
	return &value, nil
}
func (r *Repository) UpdateBatchEntry(ctx context.Context, org, batchID, id platform.ID, expected int64, set bson.M) error {
	result, err := r.database.Collection(batchEntriesCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "batchId": batchID, "version": expected, "archivedAt": nil}, bson.M{"$set": set})
	if err != nil {
		return fmt.Errorf("update batch entry: %w", err)
	}
	if result.MatchedCount == 0 {
		return platform.VersionConflict(expected)
	}
	return nil
}
func (r *Repository) ListBatchEntries(ctx context.Context, org, batchID platform.ID) ([]BatchEntry, error) {
	cursor, err := r.database.Collection(batchEntriesCollection).Find(ctx, bson.M{"organizationId": org, "batchId": batchID, "archivedAt": nil}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list batch entries: %w", err)
	}
	defer cursor.Close(ctx)
	values := []BatchEntry{}
	if err = cursor.All(ctx, &values); err != nil {
		return nil, fmt.Errorf("decode batch entries: %w", err)
	}
	return values, nil
}
func (r *Repository) BatchEntryTotals(ctx context.Context, org, batchID platform.ID) (int64, int64, error) {
	pipeline := mongo.Pipeline{{{Key: "$match", Value: bson.M{"organizationId": org, "batchId": batchID, "archivedAt": nil}}}, {{Key: "$group", Value: bson.M{"_id": nil, "total": bson.M{"$sum": "$total.amountMinor"}, "count": bson.M{"$sum": 1}}}}}
	cursor, err := r.database.Collection(batchEntriesCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return 0, 0, fmt.Errorf("sum batch entries: %w", err)
	}
	defer cursor.Close(ctx)
	var rows []struct {
		Total int64 `bson:"total"`
		Count int64 `bson:"count"`
	}
	if err = cursor.All(ctx, &rows); err != nil {
		return 0, 0, err
	}
	if len(rows) == 0 {
		return 0, 0, nil
	}
	return rows[0].Total, rows[0].Count, nil
}
func (r *Repository) InsertConfirmation(ctx context.Context, value CountConfirmation) error {
	return r.Insert(ctx, batchConfirmationsCollection, value)
}
func (r *Repository) ListConfirmations(ctx context.Context, org, batchID platform.ID) ([]CountConfirmation, error) {
	cursor, err := r.database.Collection(batchConfirmationsCollection).Find(ctx, bson.M{"organizationId": org, "batchId": batchID}, options.Find().SetSort(bson.D{{Key: "confirmedAt", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list batch confirmations: %w", err)
	}
	defer cursor.Close(ctx)
	values := []CountConfirmation{}
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return values, nil
}
func (r *Repository) DeleteConfirmations(ctx context.Context, org, batchID platform.ID) error {
	_, err := r.database.Collection(batchConfirmationsCollection).DeleteMany(ctx, bson.M{"organizationId": org, "batchId": batchID})
	if err != nil {
		return fmt.Errorf("reset batch confirmations: %w", err)
	}
	return nil
}
func (r *Repository) InsertBatchEvent(ctx context.Context, value BatchEvent) error {
	return r.Insert(ctx, batchEventsCollection, value)
}
func (r *Repository) ListBatchEvents(ctx context.Context, org, batchID platform.ID) ([]BatchEvent, error) {
	cursor, err := r.database.Collection(batchEventsCollection).Find(ctx, bson.M{"organizationId": org, "batchId": batchID}, options.Find().SetSort(bson.D{{Key: "sequence", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list batch events: %w", err)
	}
	defer cursor.Close(ctx)
	values := []BatchEvent{}
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return values, nil
}
