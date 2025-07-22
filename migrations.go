package cargo

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Migration represents a database migration
type Migration struct {
	ID          string
	Description string
	Up          func(ctx context.Context, db *mongo.Database) error
	Down        func(ctx context.Context, db *mongo.Database) error
}

// MigrationRecord represents a migration record in the database
type MigrationRecord struct {
	ID          string    `bson:"_id"`
	Description string    `bson:"description"`
	AppliedAt   time.Time `bson:"applied_at"`
	Version     int       `bson:"version"`
}

// MigrationManager handles database migrations
type MigrationManager struct {
	app        *App
	migrations []Migration
	collection *mongo.Collection
}

// NewMigrationManager creates a new migration manager
func (a *App) NewMigrationManager() *MigrationManager {
	collection := a.getDatabase().Collection("_migrations")
	return &MigrationManager{
		app:        a,
		migrations: make([]Migration, 0),
		collection: collection,
	}
}

// Add adds a migration to the manager
func (mm *MigrationManager) Add(migration Migration) {
	mm.migrations = append(mm.migrations, migration)
}

// RunMigrations executes all pending migrations
func (mm *MigrationManager) RunMigrations(ctx context.Context) error {
	// Ensure migration collection has proper index
	if err := mm.ensureMigrationIndex(ctx); err != nil {
		return WrapError(err, 500, "failed to create migration index")
	}

	appliedMigrations, err := mm.getAppliedMigrations(ctx)
	if err != nil {
		return WrapError(err, 500, "failed to get applied migrations")
	}

	for _, migration := range mm.migrations {
		if mm.isMigrationApplied(migration.ID, appliedMigrations) {
			log.Printf("Migration %s already applied, skipping", migration.ID)
			continue
		}

		log.Printf("Running migration: %s - %s", migration.ID, migration.Description)

		// Run the migration in a transaction for safety
		tm, err := mm.app.NewTransactionManager()
		if err != nil {
			return WrapError(err, 500, "failed to create transaction manager")
		}

		err = tm.WithTransaction(ctx, func(sc mongo.SessionContext) error {
			// Execute the migration
			if err := migration.Up(sc, mm.app.getDatabase()); err != nil {
				return err
			}

			// Record the migration as applied
			record := MigrationRecord{
				ID:          migration.ID,
				Description: migration.Description,
				AppliedAt:   time.Now(),
				Version:     len(appliedMigrations) + 1,
			}

			_, err := mm.collection.InsertOne(sc, record)
			return err
		})

		if err != nil {
			return WrapError(err, 500, fmt.Sprintf("migration %s failed", migration.ID))
		}

		log.Printf("Migration %s completed successfully", migration.ID)
	}

	return nil
}

// Rollback rolls back the last applied migration
func (mm *MigrationManager) Rollback(ctx context.Context) error {
	// Get the last applied migration
	lastMigration, err := mm.getLastAppliedMigration(ctx)
	if err != nil {
		return WrapError(err, 500, "failed to get last migration")
	}

	if lastMigration == nil {
		return NewValidationError("no migrations to rollback")
	}

	// Find the migration definition
	var targetMigration *Migration
	for _, migration := range mm.migrations {
		if migration.ID == lastMigration.ID {
			targetMigration = &migration
			break
		}
	}

	if targetMigration == nil {
		return NewValidationError(fmt.Sprintf("migration definition not found for %s", lastMigration.ID))
	}

	if targetMigration.Down == nil {
		return NewValidationError(fmt.Sprintf("migration %s does not support rollback", lastMigration.ID))
	}

	log.Printf("Rolling back migration: %s - %s", targetMigration.ID, targetMigration.Description)

	// Run the rollback in a transaction
	tm, err := mm.app.NewTransactionManager()
	if err != nil {
		return WrapError(err, 500, "failed to create transaction manager")
	}

	err = tm.WithTransaction(ctx, func(sc mongo.SessionContext) error {
		// Execute the rollback
		if err := targetMigration.Down(sc, mm.app.getDatabase()); err != nil {
			return err
		}

		// Remove the migration record
		_, err := mm.collection.DeleteOne(sc, bson.M{"_id": lastMigration.ID})
		return err
	})

	if err != nil {
		return WrapError(err, 500, fmt.Sprintf("rollback of migration %s failed", targetMigration.ID))
	}

	log.Printf("Migration %s rolled back successfully", targetMigration.ID)
	return nil
}

// GetMigrationStatus returns the current migration status
func (mm *MigrationManager) GetMigrationStatus(ctx context.Context) ([]MigrationStatus, error) {
	appliedMigrations, err := mm.getAppliedMigrations(ctx)
	if err != nil {
		return nil, WrapError(err, 500, "failed to get applied migrations")
	}

	var status []MigrationStatus
	for _, migration := range mm.migrations {
		migrationStatus := MigrationStatus{
			ID:          migration.ID,
			Description: migration.Description,
			Applied:     mm.isMigrationApplied(migration.ID, appliedMigrations),
		}

		// Find application details if applied
		for _, applied := range appliedMigrations {
			if applied.ID == migration.ID {
				migrationStatus.AppliedAt = &applied.AppliedAt
				migrationStatus.Version = applied.Version
				break
			}
		}

		status = append(status, migrationStatus)
	}

	return status, nil
}

// MigrationStatus represents the status of a migration
type MigrationStatus struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Applied     bool       `json:"applied"`
	AppliedAt   *time.Time `json:"applied_at,omitempty"`
	Version     int        `json:"version"`
}

// ensureMigrationIndex creates an index on the migration collection
func (mm *MigrationManager) ensureMigrationIndex(ctx context.Context) error {
	indexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "version", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	}

	_, err := mm.collection.Indexes().CreateOne(ctx, indexModel)
	return err
}

// getAppliedMigrations returns all applied migrations
func (mm *MigrationManager) getAppliedMigrations(ctx context.Context) ([]MigrationRecord, error) {
	cursor, err := mm.collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "version", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var migrations []MigrationRecord
	if err = cursor.All(ctx, &migrations); err != nil {
		return nil, err
	}

	return migrations, nil
}

// getLastAppliedMigration returns the last applied migration
func (mm *MigrationManager) getLastAppliedMigration(ctx context.Context) (*MigrationRecord, error) {
	var migration MigrationRecord
	err := mm.collection.FindOne(
		ctx,
		bson.M{},
		options.FindOne().SetSort(bson.D{{Key: "version", Value: -1}}),
	).Decode(&migration)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return &migration, nil
}

// isMigrationApplied checks if a migration has been applied
func (mm *MigrationManager) isMigrationApplied(migrationID string, appliedMigrations []MigrationRecord) bool {
	for _, applied := range appliedMigrations {
		if applied.ID == migrationID {
			return true
		}
	}
	return false
}

// Common migration helpers

// CreateCollectionMigration creates a migration that creates a collection with indexes
func CreateCollectionMigration(id, description, collectionName string, indexes []mongo.IndexModel) Migration {
	return Migration{
		ID:          id,
		Description: description,
		Up: func(ctx context.Context, db *mongo.Database) error {
			// Create the collection
			err := db.CreateCollection(ctx, collectionName)
			if err != nil {
				return err
			}

			// Create indexes if provided
			if len(indexes) > 0 {
				collection := db.Collection(collectionName)
				_, err = collection.Indexes().CreateMany(ctx, indexes)
				if err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(ctx context.Context, db *mongo.Database) error {
			return db.Collection(collectionName).Drop(ctx)
		},
	}
}

// AddIndexMigration creates a migration that adds an index to a collection
func AddIndexMigration(id, description, collectionName string, index mongo.IndexModel) Migration {
	return Migration{
		ID:          id,
		Description: description,
		Up: func(ctx context.Context, db *mongo.Database) error {
			collection := db.Collection(collectionName)
			_, err := collection.Indexes().CreateOne(ctx, index)
			return err
		},
		Down: func(ctx context.Context, db *mongo.Database) error {
			if index.Options != nil && index.Options.Name != nil {
				collection := db.Collection(collectionName)
				_, err := collection.Indexes().DropOne(ctx, *index.Options.Name)
				return err
			}
			return NewValidationError("cannot rollback index creation without index name")
		},
	}
}

// DataMigration creates a migration for data transformation
func DataMigration(id, description string, upFn, downFn func(ctx context.Context, db *mongo.Database) error) Migration {
	return Migration{
		ID:          id,
		Description: description,
		Up:          upFn,
		Down:        downFn,
	}
}
