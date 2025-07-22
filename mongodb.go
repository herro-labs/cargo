package cargo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDB operations that are specific to MongoDB and provide advanced features

// BulkWriter provides bulk write operations for better performance
type BulkWriter struct {
	collection *mongo.Collection
	operations []mongo.WriteModel
	opts       *options.BulkWriteOptions
}

// NewBulkWriter creates a new bulk writer for a collection
func (a *App) NewBulkWriter(collectionName string) *BulkWriter {
	collection := a.getDatabase().Collection(collectionName)
	return &BulkWriter{
		collection: collection,
		operations: make([]mongo.WriteModel, 0),
		opts:       options.BulkWrite(),
	}
}

// AddInsert adds an insert operation to the bulk writer
func (bw *BulkWriter) AddInsert(document interface{}) *BulkWriter {
	model := mongo.NewInsertOneModel().SetDocument(document)
	bw.operations = append(bw.operations, model)
	return bw
}

// AddUpdate adds an update operation to the bulk writer
func (bw *BulkWriter) AddUpdate(filter, update interface{}) *BulkWriter {
	model := mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update)
	bw.operations = append(bw.operations, model)
	return bw
}

// AddReplace adds a replace operation to the bulk writer
func (bw *BulkWriter) AddReplace(filter, replacement interface{}) *BulkWriter {
	model := mongo.NewReplaceOneModel().SetFilter(filter).SetReplacement(replacement)
	bw.operations = append(bw.operations, model)
	return bw
}

// AddDelete adds a delete operation to the bulk writer
func (bw *BulkWriter) AddDelete(filter interface{}) *BulkWriter {
	model := mongo.NewDeleteOneModel().SetFilter(filter)
	bw.operations = append(bw.operations, model)
	return bw
}

// Execute executes all bulk operations
func (bw *BulkWriter) Execute(ctx context.Context) (*mongo.BulkWriteResult, error) {
	if len(bw.operations) == 0 {
		return nil, NewValidationError("no operations to execute")
	}

	result, err := bw.collection.BulkWrite(ctx, bw.operations, bw.opts)
	if err != nil {
		return nil, WrapError(err, 500, "bulk write operation failed")
	}

	return result, nil
}

// AggregationBuilder provides a fluent interface for building aggregation pipelines
type AggregationBuilder struct {
	collection *mongo.Collection
	pipeline   []bson.M
	opts       *options.AggregateOptions
}

// NewAggregationBuilder creates a new aggregation builder
func (a *App) NewAggregationBuilder(collectionName string) *AggregationBuilder {
	collection := a.getDatabase().Collection(collectionName)
	return &AggregationBuilder{
		collection: collection,
		pipeline:   make([]bson.M, 0),
		opts:       options.Aggregate(),
	}
}

// Match adds a $match stage to the pipeline
func (ab *AggregationBuilder) Match(filter bson.M) *AggregationBuilder {
	ab.pipeline = append(ab.pipeline, bson.M{"$match": filter})
	return ab
}

// Project adds a $project stage to the pipeline
func (ab *AggregationBuilder) Project(projection bson.M) *AggregationBuilder {
	ab.pipeline = append(ab.pipeline, bson.M{"$project": projection})
	return ab
}

// Group adds a $group stage to the pipeline
func (ab *AggregationBuilder) Group(group bson.M) *AggregationBuilder {
	ab.pipeline = append(ab.pipeline, bson.M{"$group": group})
	return ab
}

// Sort adds a $sort stage to the pipeline
func (ab *AggregationBuilder) Sort(sort bson.M) *AggregationBuilder {
	ab.pipeline = append(ab.pipeline, bson.M{"$sort": sort})
	return ab
}

// Limit adds a $limit stage to the pipeline
func (ab *AggregationBuilder) Limit(limit int64) *AggregationBuilder {
	ab.pipeline = append(ab.pipeline, bson.M{"$limit": limit})
	return ab
}

// Skip adds a $skip stage to the pipeline
func (ab *AggregationBuilder) Skip(skip int64) *AggregationBuilder {
	ab.pipeline = append(ab.pipeline, bson.M{"$skip": skip})
	return ab
}

// Lookup adds a $lookup stage for joining collections
func (ab *AggregationBuilder) Lookup(from, localField, foreignField, as string) *AggregationBuilder {
	lookup := bson.M{
		"$lookup": bson.M{
			"from":         from,
			"localField":   localField,
			"foreignField": foreignField,
			"as":           as,
		},
	}
	ab.pipeline = append(ab.pipeline, lookup)
	return ab
}

// Unwind adds an $unwind stage to the pipeline
func (ab *AggregationBuilder) Unwind(path string) *AggregationBuilder {
	ab.pipeline = append(ab.pipeline, bson.M{"$unwind": path})
	return ab
}

// AddStage adds a custom stage to the pipeline
func (ab *AggregationBuilder) AddStage(stage bson.M) *AggregationBuilder {
	ab.pipeline = append(ab.pipeline, stage)
	return ab
}

// Execute executes the aggregation pipeline
func (ab *AggregationBuilder) Execute(ctx context.Context) (*mongo.Cursor, error) {
	if len(ab.pipeline) == 0 {
		return nil, NewValidationError("aggregation pipeline cannot be empty")
	}

	cursor, err := ab.collection.Aggregate(ctx, ab.pipeline, ab.opts)
	if err != nil {
		return nil, WrapError(err, 500, "aggregation execution failed")
	}

	return cursor, nil
}

// TransactionManager handles MongoDB transactions
type TransactionManager struct {
	client  *mongo.Client
	session mongo.Session
}

// NewTransactionManager creates a new transaction manager
func (a *App) NewTransactionManager() (*TransactionManager, error) {
	session, err := a.mongoClient.StartSession()
	if err != nil {
		return nil, WrapError(err, 500, "failed to start session")
	}

	return &TransactionManager{
		client:  a.mongoClient,
		session: session,
	}, nil
}

// WithTransaction executes a function within a transaction
func (tm *TransactionManager) WithTransaction(ctx context.Context, fn func(mongo.SessionContext) error) error {
	defer tm.session.EndSession(ctx)

	_, err := tm.session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		return nil, fn(sc)
	})

	if err != nil {
		return WrapError(err, 500, "transaction failed")
	}

	return nil
}

// IndexManager handles index creation and management
type IndexManager struct {
	collection *mongo.Collection
}

// NewIndexManager creates a new index manager for a collection
func (a *App) NewIndexManager(collectionName string) *IndexManager {
	collection := a.getDatabase().Collection(collectionName)
	return &IndexManager{collection: collection}
}

// CreateIndex creates a single index
func (im *IndexManager) CreateIndex(ctx context.Context, keys bson.M, opts ...*options.IndexOptions) (string, error) {
	model := mongo.IndexModel{
		Keys: keys,
	}

	if len(opts) > 0 {
		model.Options = opts[0]
	}

	result, err := im.collection.Indexes().CreateOne(ctx, model)
	if err != nil {
		return "", WrapError(err, 500, "failed to create index")
	}

	return result, nil
}

// CreateIndexes creates multiple indexes
func (im *IndexManager) CreateIndexes(ctx context.Context, models []mongo.IndexModel) ([]string, error) {
	results, err := im.collection.Indexes().CreateMany(ctx, models)
	if err != nil {
		return nil, WrapError(err, 500, "failed to create indexes")
	}

	return results, nil
}

// DropIndex drops an index by name
func (im *IndexManager) DropIndex(ctx context.Context, indexName string) error {
	_, err := im.collection.Indexes().DropOne(ctx, indexName)
	if err != nil {
		return WrapError(err, 500, "failed to drop index")
	}

	return nil
}

// ListIndexes lists all indexes on the collection
func (im *IndexManager) ListIndexes(ctx context.Context) ([]bson.M, error) {
	cursor, err := im.collection.Indexes().List(ctx)
	if err != nil {
		return nil, WrapError(err, 500, "failed to list indexes")
	}
	defer cursor.Close(ctx)

	var indexes []bson.M
	if err = cursor.All(ctx, &indexes); err != nil {
		return nil, WrapError(err, 500, "failed to decode indexes")
	}

	return indexes, nil
}

// HealthChecker provides MongoDB health checking capabilities
type HealthChecker struct {
	client *mongo.Client
}

// NewHealthChecker creates a new health checker
func (a *App) NewHealthChecker() *HealthChecker {
	return &HealthChecker{client: a.mongoClient}
}

// Ping checks if MongoDB is accessible
func (hc *HealthChecker) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := hc.client.Ping(ctx, nil)
	if err != nil {
		return WrapError(err, 503, "MongoDB health check failed")
	}

	return nil
}

// GetStats returns MongoDB server statistics
func (hc *HealthChecker) GetStats(ctx context.Context, dbName string) (bson.M, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result := hc.client.Database(dbName).RunCommand(ctx, bson.M{"dbStats": 1})

	var stats bson.M
	if err := result.Decode(&stats); err != nil {
		return nil, WrapError(err, 500, "failed to get database stats")
	}

	return stats, nil
}

// Enhanced Query Builder extending the basic Query
type EnhancedQuery struct {
	*Query
	collection *mongo.Collection
}

// NewEnhancedQuery creates an enhanced query builder
func (a *App) NewEnhancedQuery(collectionName string) *EnhancedQuery {
	collection := a.getDatabase().Collection(collectionName)
	return &EnhancedQuery{
		Query:      NewQuery(),
		collection: collection,
	}
}

// WhereID adds an _id filter (handles both string and ObjectID)
func (eq *EnhancedQuery) WhereID(id string) *EnhancedQuery {
	if oid, err := primitive.ObjectIDFromHex(id); err == nil {
		eq.Filter(bson.M{"_id": oid})
	} else {
		eq.Filter(bson.M{"_id": id})
	}
	return eq
}

// WhereIn adds an $in filter
func (eq *EnhancedQuery) WhereIn(field string, values []interface{}) *EnhancedQuery {
	eq.Filter(bson.M{field: bson.M{"$in": values}})
	return eq
}

// WhereNotIn adds a $nin filter
func (eq *EnhancedQuery) WhereNotIn(field string, values []interface{}) *EnhancedQuery {
	eq.Filter(bson.M{field: bson.M{"$nin": values}})
	return eq
}

// WhereExists adds an $exists filter
func (eq *EnhancedQuery) WhereExists(field string, exists bool) *EnhancedQuery {
	eq.Filter(bson.M{field: bson.M{"$exists": exists}})
	return eq
}

// WhereRegex adds a regex filter
func (eq *EnhancedQuery) WhereRegex(field, pattern string) *EnhancedQuery {
	eq.Filter(bson.M{field: bson.M{"$regex": pattern, "$options": "i"}})
	return eq
}

// WhereGTE adds a $gte (greater than or equal) filter
func (eq *EnhancedQuery) WhereGTE(field string, value interface{}) *EnhancedQuery {
	eq.Filter(bson.M{field: bson.M{"$gte": value}})
	return eq
}

// WhereLTE adds a $lte (less than or equal) filter
func (eq *EnhancedQuery) WhereLTE(field string, value interface{}) *EnhancedQuery {
	eq.Filter(bson.M{field: bson.M{"$lte": value}})
	return eq
}

// WhereDateRange adds a date range filter
func (eq *EnhancedQuery) WhereDateRange(field string, start, end time.Time) *EnhancedQuery {
	eq.Filter(bson.M{field: bson.M{"$gte": start, "$lte": end}})
	return eq
}

// Count returns the count of documents matching the query
func (eq *EnhancedQuery) Count(ctx context.Context) (int64, error) {
	count, err := eq.collection.CountDocuments(ctx, eq.GetFilter())
	if err != nil {
		return 0, WrapError(err, 500, "count operation failed")
	}
	return count, nil
}

// FindOne finds a single document
func (eq *EnhancedQuery) FindOne(ctx context.Context, result interface{}) error {
	opts := options.FindOne()
	if eq.GetOptions().Sort != nil {
		opts.SetSort(eq.GetOptions().Sort)
	}

	err := eq.collection.FindOne(ctx, eq.GetFilter(), opts).Decode(result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return NewNotFoundError("document not found")
		}
		return WrapError(err, 500, "find operation failed")
	}
	return nil
}

// Find finds multiple documents
func (eq *EnhancedQuery) Find(ctx context.Context, results interface{}) error {
	cursor, err := eq.collection.Find(ctx, eq.GetFilter(), eq.GetOptions())
	if err != nil {
		return WrapError(err, 500, "find operation failed")
	}
	defer cursor.Close(ctx)

	err = cursor.All(ctx, results)
	if err != nil {
		return WrapError(err, 500, "failed to decode results")
	}

	return nil
}
