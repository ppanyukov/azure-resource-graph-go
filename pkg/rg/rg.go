// Package rg provides a simple interface to run Azure Resource Graph queries
// and unmarshall results into custom types. It is based on the modified official
// Azure SDK for Go.
package rg

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/ppanyukov/azure-resource-graph-go/pkg/rg/internal/armresourcegraph2"
	"sync"
)

// ClientOptions is reserved for future expandability of [NewClient], e.g.
// embedding [arm.ClientOptions] to allow a custom transport, retry policy,
// or cloud configuration.
type ClientOptions struct {
}

// Client is an Azure Resource Graph query client bound to a specific
// [azcore.TokenCredential]. Construct one with [NewClient] and reuse it
// across calls to [ExecClient] — construction wraps a single credential in
// a query pipeline, so building a new [Client] per call defeats the point.
//
// Client has no exported way to change its credential after construction:
// callers who need a shared, swappable default client are expected to hold
// their own package-level variable or wrapper type, the same way they would
// for any other Azure SDK client.
type Client struct {
	armClient *armresourcegraph2.Client
}

// NewClient creates a new [Client] using the given credential.
func NewClient(cred azcore.TokenCredential, options *ClientOptions) (*Client, error) {
	armClient, err := armresourcegraph2.NewClient(cred, nil)
	if err != nil {
		return nil, err
	}

	return &Client{armClient: armClient}, nil
}

// defaultClient is the lazily-initialized [Client] used by [Exec], built
// at most once using [azidentity.NewDefaultAzureCredential].
var defaultClient = struct {
	once   sync.Once
	client *Client
	err    error
}{}

func getDefaultClient() (*Client, error) {
	defaultClient.once.Do(func() {
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			defaultClient.err = err
			return
		}

		defaultClient.client, defaultClient.err = NewClient(cred, nil)
	})

	return defaultClient.client, defaultClient.err
}

// ExecOptions is reserved for future expandability, e.g. providing subscription list.
type ExecOptions struct {
}

// Exec executes Azure Resource Graph query and returns rows from the result unmarshalled as an array of T.
//
// This function uses a shared, lazily-initialized [Client] built with the Azure Token Credential
// obtained by calling official Azure SDK for Go function [azidentity.NewDefaultAzureCredential].
//
// Use [ExecClient] instead if you need to supply your own [azcore.TokenCredential].
//
// Example:
//
//	type record struct {
//		Name string
//		Type string
//	}
//
//	items, err := rg.Exec[record](context.Background(), "resources | project name, type | order by name, type", nil)
//	if err != nil {
//		panic(err)
//	}
//
//	for _, item := range items {
//		fmt.printf("%s, %s\n", item.Name, item.Type)
//  }
func Exec[T any](ctx context.Context, query string, options *ExecOptions) ([]T, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}

	return ExecClient[T](client, ctx, query, options)
}

// ExecClient executes Azure Resource Graph query using the given [Client] and returns rows
// from the result unmarshalled as an array of T.
//
// Use this instead of [Exec] when you need to supply your own [azcore.TokenCredential] rather
// than relying on the package's shared default. Construct the [Client] once with [NewClient]
// and reuse it across calls.
//
// Example:
//
//	r, err := rg.NewClient(myCredential, nil)
//	if err != nil {
//		panic(err)
//	}
//
//	items, err := rg.ExecClient[record](r, context.Background(), "resources | project name, type", nil)
func ExecClient[T any](r *Client, ctx context.Context, query string, options *ExecOptions) ([]T, error) {
	queryRequest := armresourcegraph2.QueryRequest{
		Query: &query,
	}

	return armresourcegraph2.ResourcesAll2[T](r.armClient, ctx, queryRequest)
}
