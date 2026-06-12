package database

import "context"

type TransactionRunner interface {
	RunInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}
