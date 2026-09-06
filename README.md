# azure-resource-graph-go

Simple, convenient way to exec Azure Resource Graph queries in Go with automatic credentials, automatic paging, and **unmarshalling results into user-specified data types `T`** just like `json.Unmarshal` does.

The main use case is for various scripts and utilities which need to query information about Azure infrastructure across subscriptions and is a substitute for a lot of the official Azure SDK for Go, and is much simpler and faster to use too. 

The top-level function `rg.Exec` just takes query text as an argument and uses an automatic internal package-maintained shared default Azure Token Credential (there are ways to customise this, see "Notes on authentication" below). 

(*There is currently no way to customise other query options, e.g. the list of subscriptions against which the query runs. This may be added later if needed.*)

See `examples` section and directory for all samples of usage.

### Why

**Why not use `armresourcegraph` package from Azure SDK directly?**

This package provides multiple advantages:

* **Much simpler usage**. The interface is simplified for the most common use cases. The native SDK has many data types, pointers, etc.

* **Automatic credentials**. No need to setup Azure authentication for simple use cases. Own credentials can still be supplied when needed.

* **Automatic paging**. All results are returned in an array directly, no need to have boilerplate code to handle paging.

* **Efficient automatic direct unmarshall into user type `T`**. Native SDK returns types of `any`. Not only the user would need to handle this, but this also would mean double-unmarshall: first marshal the native result into JSON, then back into user type. This package avoids this by directly unmarshalling raw results in the user type.


**Why use Resource Graph and not use resource-specific ARM clients from Azure SDK?**

The official Azure SDK for Go is difficult to use:

* Separate client for each kind of resource, needs to be imported separately.
* Clients work on subscription level. To list things across subscription requires extra steps and is also slow.
* Slow as they pull down a lot of stuff that's probably not needed.
* Complex data types which are difficult to use, lots of pointers.
* Not to mention paging which needs to be handled by the user.

It's much better to use Azure Resource Graph in many cases:

* Almost all data provided by ARM clients can be obtained using Resource Graph.
* Works across subscriptions by default.
* Much faster.
* Allows advanced filtering, projection, joins, sort order and so on in `KQL`.
* Returns data only needed in the required shape/form.

However, since the official Azure SDK for Go Resource Graph client is also pain to use, here we have this package `rg`.


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
	// Automatic internal shared credential will be used here.
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

If you want to use your own `azcore.TokenCredential` instead of the package's default, construct a `rg.Client` with `rg.NewClient(cred, nil)` once and call `rg.ExecClient` instead of `rg.Exec`:

```go
cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
if err != nil {
	log.Fatal(err)
}

r, err := rg.NewClient(cred, nil)
if err != nil {
	log.Fatal(err)
}

items, err := rg.ExecClient[record](r, context.Background(), query, nil)
if err != nil {
	log.Fatal(err)
}
```

Reuse the same `r` across calls — it wraps a single query pipeline bound to `cred`, so building a new `Client` per call is wasteful.

For the design rationale behind this API shape (why `Exec`/`ExecClient` are free functions rather than methods, and why there's no `SetCred`/default-client override), see [ADR 1](docs/adr/0001-generic-exec-free-functions.md).

Authentication for the `rg.Exec` default credential is performed as per standard Azure SDK from the following sources:

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

