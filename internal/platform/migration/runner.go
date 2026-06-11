package migration

import (
	"context"
	"fmt"
	"sort"
	"time"
)

type Migration struct {
	Version string
	Name    string
	Apply   func(ctx context.Context) error
}

type AppliedMigration struct {
	Version   string
	Name      string
	AppliedAt time.Time
}

type Store interface {
	Ensure(ctx context.Context) error
	Has(ctx context.Context, version string) (bool, error)
	Record(ctx context.Context, migration AppliedMigration) error
}

type Runner struct {
	store Store
	now   func() time.Time
}

func NewRunner(store Store) *Runner {
	return &Runner{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

func (r *Runner) Run(ctx context.Context, migrations []Migration) ([]AppliedMigration, error) {
	if r.store == nil {
		return nil, fmt.Errorf("migration store is required")
	}
	if err := r.store.Ensure(ctx); err != nil {
		return nil, err
	}
	migrations = append([]Migration(nil), migrations...)
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	applied := make([]AppliedMigration, 0)
	for _, migration := range migrations {
		if migration.Version == "" {
			return nil, fmt.Errorf("migration version is required")
		}
		if migration.Apply == nil {
			return nil, fmt.Errorf("migration %s apply function is required", migration.Version)
		}
		exists, err := r.store.Has(ctx, migration.Version)
		if err != nil {
			return nil, err
		}
		if exists {
			continue
		}
		if err := migration.Apply(ctx); err != nil {
			return nil, fmt.Errorf("apply migration %s: %w", migration.Version, err)
		}
		record := AppliedMigration{
			Version:   migration.Version,
			Name:      migration.Name,
			AppliedAt: r.now(),
		}
		if err := r.store.Record(ctx, record); err != nil {
			return nil, err
		}
		applied = append(applied, record)
	}
	return applied, nil
}
