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

// defaultCredentialToken is the singleton shared default [azcore.TokenCredential]
// created with [azidentity.NewDefaultAzureCredential].
var defaultCredentialToken = struct {
	once sync.Once
	cred azcore.TokenCredential
	err  error
}{}

func getDefaultCredentialToken() (azcore.TokenCredential, error) {
	defaultCredentialToken.once.Do(func() {
		defaultCredentialToken.cred, defaultCredentialToken.err = azidentity.NewDefaultAzureCredential(nil)
	})

	return defaultCredentialToken.cred, defaultCredentialToken.err
}

// defaultArmClient is the singleton shared default [armresourcegraph2.Client]
// with default shared credentials
var defaultArmClient = struct {
	once      sync.Once
	armClient *armresourcegraph2.Client
	err       error
}{}

func getDefaultArmClient() (*armresourcegraph2.Client, error) {
	defaultArmClient.once.Do(func() {
		cred, err := getDefaultCredentialToken()
		if err != nil {
			defaultArmClient.err = err
			return
		}

		defaultArmClient.armClient, defaultArmClient.err = armresourcegraph2.NewClient(cred, nil)
	})

	return defaultArmClient.armClient, defaultArmClient.err
}

// ExecOptions is reserved for future expandability, e.g. providing subscription list.
type ExecOptions struct {
}

// Exec executes Azure Resource Graph query and returns rows from the result unmarshalled as an array of T.
//
// This function uses shared cached Azure Token Credential obtained by calling official Azure SDK for Go
// function [azidentity.NewDefaultAzureCredential].
//
// Example:
//
//	type record struct {
//		Name string
//		Type string
//	}
//
//	items, err := rg.Exec(context.Background, "resources | project name, type | order by name, type", nil)
//	if err != nil {
//		panic(err)
//	}
//
//	for _, item := range items {
//		fmt.printf("%s, %s\n", item.Name, item.Type)
//  }
func Exec[T any](ctx context.Context, query string, options *ExecOptions) ([]T, error) {
	queryRequest := armresourcegraph2.QueryRequest{
		Query: &query,
	}

	client, err := getDefaultArmClient()
	if err != nil {
		return nil, err
	}

	return armresourcegraph2.ResourcesAll2[T](client, ctx, queryRequest)
}

// TODO: some work in progress, keeping to keep the code as a sample
//
//type Client struct {
//	c *armresourcegraph2.Client
//	// err stores the errors related to various initializations, e.g. getting [azcore.TokenCredential].
//	err error
//}
//
//// NewDefaultClient creates new Azure Resource Graph query armClient with default shared Azure token credential.
//func NewDefaultClient() *Client {
//	result := Client{}
//	result.c, result.err = getDefaultArmClient()
//	return &result
//}
//
//// NewClient creates new Azure Resource Graph query armClient with specified Azure token credential.
//// Both [cred] and [options] can be nil, in which case default [azcore.TokenCredential] and
//// [arm.ClientOptions] provided by this package will be used.
//func NewClient(cred azcore.TokenCredential, options *arm.ClientOptions) *Client {
//	var result Client
//	if cred == nil && options == nil {
//		result.c, result.err = getDefaultArmClient()
//		return &result
//	}
//
//	if cred == nil {
//		token, err := getDefaultCredentialToken()
//		if err != nil {
//			result.err = err
//			return &result
//		}
//		cred = token
//	}
//
//	result.c, result.err = armresourcegraph2.NewClient(cred, options)
//	return &result
//}
//
//// Exec executes specified query and unmarshalls results into [out] interface like [json.Unmarshal].
//// The [out] parameter must be a pointer to a slice.
//func (client *Client) Exec(ctx context.Context, query string, options *ExecOptions, out interface{}) error {
//	queryRequest := armresourcegraph2.QueryRequest{
//		Query: &query,
//	}
//
//	return armresourcegraph2.ResourcesAll3(client.c, ctx, queryRequest, out)
//}
