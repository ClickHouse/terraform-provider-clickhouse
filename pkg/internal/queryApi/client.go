package queryApi

import (
	"context"
)

type clientImpl struct {
	baseUrl string
}

func New(BaseUrl string) (Client, error) {
	// TODO validate baseUrl is a valid URL

	return &clientImpl{
		baseUrl: BaseUrl,
	}, nil
}

func (c *clientImpl) RunQuery(ctx context.Context, query string) error {
	// todo implement me

	panic(query)

	return nil
}
