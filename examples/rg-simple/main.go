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

	// Exec the query
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
