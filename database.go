package cargo

import (
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// getCollectionName extracts collection name from protobuf message type
// Now supports protoc-gen-go-bson generated types
func getCollectionName(v interface{}) string {
	// Use BSON support for better collection name handling
	return BSON.GetBSONCollectionName(v)
}

// getDatabase returns the configured database
func (a *App) getDatabase() *mongo.Database {
	return a.mongoClient.Database(a.config.Database.MongoDB.Database)
}

// createInMongoDB inserts a document into MongoDB
func (a *App) createInMongoDB(ctx *Context, req interface{}) error {
	collection := a.getDatabase().Collection(getCollectionName(req))
	_, err := collection.InsertOne(ctx.Context, req)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("failed to create: %v", err))
	}
	return nil
}

// readFromMongoDB reads a document from MongoDB by ID
func (a *App) readFromMongoDB(ctx *Context, req interface{}) (interface{}, error) {
	// Extract ID from request - assuming there's an Id field
	v := reflect.ValueOf(req)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	idField := v.FieldByName("Id")
	if !idField.IsValid() {
		return nil, status.Error(codes.InvalidArgument, "id field not found")
	}

	idStr := idField.String()
	if idStr == "" {
		return nil, status.Error(codes.InvalidArgument, "id cannot be empty")
	}

	// Convert to ObjectID if it's a valid hex string
	var filter bson.M
	if oid, err := primitive.ObjectIDFromHex(idStr); err == nil {
		filter = bson.M{"_id": oid}
	} else {
		filter = bson.M{"id": idStr}
	}

	collection := a.getDatabase().Collection(getCollectionName(req))
	result := reflect.New(reflect.TypeOf(req).Elem()).Interface()

	err := collection.FindOne(ctx.Context, filter).Decode(result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, status.Error(codes.NotFound, "document not found")
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to read: %v", err))
	}

	return result, nil
}

// listFromMongoDB queries multiple documents from MongoDB
func (a *App) listFromMongoDB(ctx *Context, req interface{}, query *Query) (interface{}, error) {
	collection := a.getDatabase().Collection(getCollectionName(req))

	cursor, err := collection.Find(ctx.Context, query.GetFilter(), query.GetOptions())
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to query: %v", err))
	}
	defer cursor.Close(ctx.Context)

	// Create slice of the request type
	reqType := reflect.TypeOf(req)
	if reqType.Kind() == reflect.Ptr {
		reqType = reqType.Elem()
	}

	sliceType := reflect.SliceOf(reqType)
	results := reflect.New(sliceType).Elem()

	for cursor.Next(ctx.Context) {
		item := reflect.New(reqType).Interface()
		if err := cursor.Decode(item); err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to decode: %v", err))
		}
		results = reflect.Append(results, reflect.ValueOf(item).Elem())
	}

	return results.Interface(), nil
}

// deleteFromMongoDB deletes a document from MongoDB
func (a *App) deleteFromMongoDB(ctx *Context, req interface{}) error {
	// Extract ID from request
	v := reflect.ValueOf(req)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	idField := v.FieldByName("Id")
	if !idField.IsValid() {
		return status.Error(codes.InvalidArgument, "id field not found")
	}

	idStr := idField.String()
	if idStr == "" {
		return status.Error(codes.InvalidArgument, "id cannot be empty")
	}

	// Convert to ObjectID if it's a valid hex string
	var filter bson.M
	if oid, err := primitive.ObjectIDFromHex(idStr); err == nil {
		filter = bson.M{"_id": oid}
	} else {
		filter = bson.M{"id": idStr}
	}

	collection := a.getDatabase().Collection(getCollectionName(req))
	result, err := collection.DeleteOne(ctx.Context, filter)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("failed to delete: %v", err))
	}

	if result.DeletedCount == 0 {
		return status.Error(codes.NotFound, "document not found")
	}

	return nil
}

// updateInMongoDB updates a document in MongoDB
func (a *App) updateInMongoDB(ctx *Context, req interface{}) error {
	// Extract ID from request
	v := reflect.ValueOf(req)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	idField := v.FieldByName("Id")
	if !idField.IsValid() {
		return status.Error(codes.InvalidArgument, "id field not found")
	}

	idStr := idField.String()
	if idStr == "" {
		return status.Error(codes.InvalidArgument, "id cannot be empty")
	}

	// Convert to ObjectID if it's a valid hex string
	var filter bson.M
	if oid, err := primitive.ObjectIDFromHex(idStr); err == nil {
		filter = bson.M{"_id": oid}
	} else {
		filter = bson.M{"id": idStr}
	}

	update := bson.M{"$set": req}

	collection := a.getDatabase().Collection(getCollectionName(req))
	result, err := collection.UpdateOne(ctx.Context, filter, update)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("failed to update: %v", err))
	}

	if result.MatchedCount == 0 {
		return status.Error(codes.NotFound, "document not found")
	}

	return nil
}
