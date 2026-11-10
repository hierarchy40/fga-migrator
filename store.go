package fgamigrator

import (
	"context"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
)

// FindStore returns "" when no store has that name.
func FindStore(ctx context.Context, fga *client.OpenFgaClient, name string) (string, error) {
	var token *string
	for {
		res, err := fga.ListStores(ctx).Options(client.ClientListStoresOptions{ContinuationToken: token}).Execute()
		if err != nil {
			return "", err
		}
		for _, store := range res.Stores {
			if store.Name == name {
				return store.Id, nil
			}
		}
		if res.ContinuationToken == "" {
			return "", nil
		}
		token = openfga.PtrString(res.ContinuationToken)
	}
}
