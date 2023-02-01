# azure-resource-graph-go

This repo provides Go package to run queries against Azure Resource Graph in a simple, convenient way, and unmarshalls results into user-specified data types just like `json.Unmarshal` does.

The main use case is for various scripts and utilities which need to query information about Azure infrastructure across subscriptions and is a substitute for a lot of the official Azure SDK for Go, and is much simpler and faster to use too. 

Currently, there is only one top-level function `rg.Exec` which just takes query text as an argument. There are no ways to customise anything, e.g. provide the list of subscriptions against which the query runs, or provide custom Credentials Token. These may be added later if needed.



See `examples` directory for all samples of usage.



### Usage

Requirement:

* Go 1.18+ (because generics)

Install:

```
go get "github.com/ppanyukov/azure-resource-graph-go/pkg/rg"
```

Use in code:

```go
package main

import (
	"context"
	"encoding/csv"
	"github.com/jszwec/csvutil"
	"github.com/ppanyukov/azure-resource-graph-go/pkg/rg"
	"log"
	"os"
)

func main() {
	type record struct {
		Type              string
		SubscriptionName  string
		ResourceGroupName string
		Name              string
		Location          string
	}

	const query = `
		resources
		| join kind = leftouter (
			resourcecontainers
			| where type =~ "microsoft.resources/subscriptions"
			| project subscriptionId, subscriptionName=name
		) on subscriptionId
		| project type, subscriptionName, resourceGroupName=resourceGroup, name, location
		| order by type asc, subscriptionName asc, resourceGroupName asc, name asc, location asc
	`

	// Exec the query. This returns results unmarshalled as []record.
	items, err := rg.Exec[record](context.Background(), query, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Print out as CSV
	w := csv.NewWriter(os.Stdout)
	if err := csvutil.NewEncoder(w).Encode(items); err != nil {
		log.Fatal(err)
	}
	w.Flush()
}
```

### Notes on authentication

The method `rg.Exec` uses a cached shared Azure Token Credential maintained by the package created by `azidentity.NewDefaultAzureCredential()`. Repeated calls to `rg.Exec` reuse this token credential.

Authentication is performed as per standard Azure SDK from the following sources:

* EnvironmentCredential
* ManagedIdentityCredentialOptions
* AzureCLICredential

If you already have Azure CLI installed and have logged in there, all programs using this package should run without doing anything special.

The `EnvironmentCredential` uses standard environment variables:

* AZURE_CLIENT_ID
* AZURE_TENANT_ID
* AZURE_CLIENT_SECRET
* etc

For full up-to-date list of env vars etc see:

* https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/azidentity#readme-authenticate-with-defaultazurecredential
* https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/azidentity#DefaultAzureCredential
* https://github.com/Azure/azure-sdk-for-go/tree/main/sdk/azidentity#environment-variables



### Motivation

The official Azure SDK for Go is difficult to use for the following reasons:

* Regular ARM clients a supplied for each kind of resource and each needs to be imported separately.
* Regular ARM clients work on subscription level. To list things across subscription requires extra steps and is also slow.
* Regular ARM clients are slow as they pull down a lot of stuff that's probably not needed.

It's much better to use Azure Resource Graph because:

* Almost all data provided by ARM clients can be obtained using Resource Graph.
* It works across subscriptions by default.
* It's much faster.
* Allows advanced filtering, projection, joins, sort order and so on.

However, the official Azure SDK for Go Resource Graph client is also pain to use for these reasons:

* It doesn't provide any way to unmarshal returned data into user-provided structs.
* Paged results need to be handled by user directly, and this required understanding of what exactly is returned.
* Interface is complicated with lots of parameters etc.

So this package aims to provide a really simple way to use Azure Resource Graph, there isn't even a need to explicitly import any of Azure SDK bits. 



