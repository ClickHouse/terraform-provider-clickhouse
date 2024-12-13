package tableBuilder

import (
	"context"
)

type Builder interface {
	CreateTable(ctx context.Context, table Table) error
	GetTable(ctx context.Context, name string) (*Table, error)
	DeleteTable(ctx context.Context, name string) error
	SyncTable(ctx context.Context, table Table) error
}
