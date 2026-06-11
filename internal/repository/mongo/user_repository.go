package mongo

import (
	"context"
	"time"

	"github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const usersCollection = "users"

type UserRepository struct {
	db database.Database
}

func NewUserRepository(db database.Database) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, usr user.User) error {
	doc := userDocumentFromDomain(usr)
	return r.db.InsertOne(ctx, usersCollection, doc, database.WriteOptions{
		InvalidateKeys: []string{userIDKey(usr.ID), userEmailKey(usr.Email)},
	})
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*user.User, error) {
	var doc userDocument
	if err := r.db.FindOne(ctx, usersCollection, bson.M{"_id": id}, &doc, database.ReadOptions{
		CacheKey:   userIDKey(id),
		CacheTTL:   10 * time.Minute,
		LockOnMiss: true,
	}); err != nil {
		return nil, err
	}
	usr := doc.toDomain()
	return &usr, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	normalized := user.NormalizeEmail(email)
	var doc userDocument
	if err := r.db.FindOne(ctx, usersCollection, bson.M{"email": normalized}, &doc, database.ReadOptions{
		CacheKey:   userEmailKey(normalized),
		CacheTTL:   10 * time.Minute,
		LockOnMiss: true,
	}); err != nil {
		return nil, err
	}
	usr := doc.toDomain()
	return &usr, nil
}

func (r *UserRepository) EnsureRole(ctx context.Context, userID string, role user.Role, updatedAt time.Time) error {
	return r.db.UpdateOne(ctx, usersCollection, bson.M{"_id": userID}, bson.M{
		"$addToSet": bson.M{"roles": role},
		"$set":      bson.M{"updated_at": updatedAt},
	}, database.WriteOptions{
		LockKey:        userIDKey(userID),
		InvalidateKeys: []string{userIDKey(userID)},
	})
}

func (r *UserRepository) UpdateLastLogin(ctx context.Context, userID string, at time.Time) error {
	return r.db.UpdateOne(ctx, usersCollection, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"last_login_at": at,
			"updated_at":    at,
		},
	}, database.WriteOptions{
		LockKey:        userIDKey(userID),
		InvalidateKeys: []string{userIDKey(userID)},
	})
}

type userDocument struct {
	ID                  string      `bson:"_id" json:"id"`
	Email               string      `bson:"email" json:"email"`
	PasswordHash        string      `bson:"password_hash" json:"password_hash"`
	Name                string      `bson:"name" json:"name"`
	Roles               []user.Role `bson:"roles" json:"roles"`
	Status              user.Status `bson:"status" json:"status"`
	FailedLoginAttempts int         `bson:"failed_login_attempts" json:"failed_login_attempts"`
	LockedUntil         *time.Time  `bson:"locked_until,omitempty" json:"locked_until,omitempty"`
	CreatedAt           time.Time   `bson:"created_at" json:"created_at"`
	UpdatedAt           time.Time   `bson:"updated_at" json:"updated_at"`
	LastLoginAt         *time.Time  `bson:"last_login_at,omitempty" json:"last_login_at,omitempty"`
}

func userDocumentFromDomain(usr user.User) userDocument {
	return userDocument{
		ID:                  usr.ID,
		Email:               user.NormalizeEmail(usr.Email),
		PasswordHash:        usr.PasswordHash,
		Name:                usr.Name,
		Roles:               usr.Roles,
		Status:              usr.Status,
		FailedLoginAttempts: usr.FailedLoginAttempts,
		LockedUntil:         usr.LockedUntil,
		CreatedAt:           usr.CreatedAt,
		UpdatedAt:           usr.UpdatedAt,
		LastLoginAt:         usr.LastLoginAt,
	}
}

func (d userDocument) toDomain() user.User {
	return user.User{
		ID:                  d.ID,
		Email:               d.Email,
		PasswordHash:        d.PasswordHash,
		Name:                d.Name,
		Roles:               d.Roles,
		Status:              d.Status,
		FailedLoginAttempts: d.FailedLoginAttempts,
		LockedUntil:         d.LockedUntil,
		CreatedAt:           d.CreatedAt,
		UpdatedAt:           d.UpdatedAt,
		LastLoginAt:         d.LastLoginAt,
	}
}
