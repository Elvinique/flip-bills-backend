package mongo

// TravelCacheRepository stores raw, variable partner API payloads in MongoDB.
// Bus operators (GIGM, ABC) and flight GDS (Amadeus) return deeply nested JSON
// with schemas that change per partner — MongoDB is the right store for this.

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SearchCacheDoc is a TTL-backed document holding raw operator search results.
type SearchCacheDoc struct {
	ID        primitive.ObjectID       `bson:"_id,omitempty"`
	CacheKey  string                   `bson:"cache_key"` // e.g. "bus:lagos:abuja:2026-06-01"
	Results   []map[string]interface{} `bson:"results"`
	CachedAt  time.Time                `bson:"cached_at"`
	ExpiresAt time.Time                `bson:"expires_at"`
}

// OperatorPayloadDoc stores the full raw operator booking response for audit.
type OperatorPayloadDoc struct {
	ID           primitive.ObjectID     `bson:"_id,omitempty"`
	BookingID    string                 `bson:"booking_id"`
	OperatorCode string                 `bson:"operator_code"`
	RawRequest   map[string]interface{} `bson:"raw_request"`
	RawResponse  map[string]interface{} `bson:"raw_response"`
	CreatedAt    time.Time              `bson:"created_at"`
}

type TravelCacheRepository struct {
	searchCache      *mongo.Collection
	operatorPayloads *mongo.Collection
}

func NewTravelCacheRepository(db *mongo.Database) *TravelCacheRepository {
	repo := &TravelCacheRepository{
		searchCache:      db.Collection("travel_search_cache"),
		operatorPayloads: db.Collection("operator_payloads"),
	}
	repo.ensureIndexes(context.Background())
	return repo
}

// ensureIndexes creates TTL and lookup indexes on startup.
func (r *TravelCacheRepository) ensureIndexes(ctx context.Context) {
	// TTL index — MongoDB auto-deletes expired search cache docs.
	r.searchCache.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	})
	r.searchCache.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "cache_key", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	r.operatorPayloads.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "booking_id", Value: 1}},
	})
}

// GetSearchCache retrieves a cached search result if it exists and hasn't expired.
func (r *TravelCacheRepository) GetSearchCache(ctx context.Context, key string) (*SearchCacheDoc, error) {
	var doc SearchCacheDoc
	err := r.searchCache.FindOne(ctx, bson.M{
		"cache_key":  key,
		"expires_at": bson.M{"$gt": time.Now()},
	}).Decode(&doc)
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// SetSearchCache stores search results for the given cache key with a 10-minute TTL.
// Operator inventory changes fast — we don't cache longer than this.
func (r *TravelCacheRepository) SetSearchCache(ctx context.Context, key string, results []map[string]interface{}) error {
	doc := SearchCacheDoc{
		CacheKey:  key,
		Results:   results,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	opts := options.Replace().SetUpsert(true)
	_, err := r.searchCache.ReplaceOne(ctx, bson.M{"cache_key": key}, doc, opts)
	return err
}

// SaveOperatorPayload stores the full raw request/response for a booking for audit.
func (r *TravelCacheRepository) SaveOperatorPayload(ctx context.Context, doc OperatorPayloadDoc) error {
	doc.CreatedAt = time.Now()
	_, err := r.operatorPayloads.InsertOne(ctx, doc)
	return err
}
