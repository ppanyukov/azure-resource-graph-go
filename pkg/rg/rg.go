//go:build go1.18
// +build go1.18

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

// RgClient is an Azure Resource Graph query client bound to a specific
// [azcore.TokenCredential]. Construct one with [NewRgClient] and reuse it
// across calls to [ExecClient] — construction wraps a single credential in
// a query pipeline, so building a new [RgClient] per call defeats the point.
//
// RgClient has no exported way to change its credential after construction:
// callers who need a shared, swappable default client are expected to hold
// their own package-level variable or wrapper type, the same way they would
// for any other Azure SDK client.
type RgClient struct {
	armClient *armresourcegraph2.Client
}

// NewRgClient creates a new [RgClient] using the given credential.
func NewRgClient(cred azcore.TokenCredential) (*RgClient, error) {
	armClient, err := armresourcegraph2.NewClient(cred, nil)
	if err != nil {
		return nil, err
	}

	return &RgClient{armClient: armClient}, nil
}

// defaultClient is the lazily-initialized [RgClient] used by [Exec], built
// at most once using [azidentity.NewDefaultAzureCredential].
var defaultClient = struct {
	once   sync.Once
	client *RgClient
	err    error
}{}

func getDefaultClient() (*RgClient, error) {
	defaultClient.once.Do(func() {
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			defaultClient.err = err
			return
		}

		defaultClient.client, defaultClient.err = NewRgClient(cred)
	})

	return defaultClient.client, defaultClient.err
}

// ExecOptions is reserved for future expandability, e.g. providing subscription list.
type ExecOptions struct {
}

// Exec executes Azure Resource Graph query and returns rows from the result unmarshalled as an array of T.
//
// This function uses a shared, lazily-initialized [RgClient] built with the Azure Token Credential
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

// ExecClient executes Azure Resource Graph query using the given [RgClient] and returns rows
// from the result unmarshalled as an array of T.
//
// Use this instead of [Exec] when you need to supply your own [azcore.TokenCredential] rather
// than relying on the package's shared default. Construct the [RgClient] once with [NewRgClient]
// and reuse it across calls.
//
// Example:
//
//	r, err := rg.NewRgClient(myCredential)
//	if err != nil {
//		panic(err)
//	}
//
//	items, err := rg.ExecClient[record](r, context.Background(), "resources | project name, type", nil)
func ExecClient[T any](r *RgClient, ctx context.Context, query string, options *ExecOptions) ([]T, error) {
	queryRequest := armresourcegraph2.QueryRequest{
		Query: &query,
	}

	return armresourcegraph2.ResourcesAll2[T](r.armClient, ctx, queryRequest)
}
