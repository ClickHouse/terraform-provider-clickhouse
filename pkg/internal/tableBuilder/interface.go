package tableBuilder

import (
	"context"
)

type Builder interface {
	CreateTable(ctx context.Context, table Table) error
}
