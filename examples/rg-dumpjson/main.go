package main

import (
	"context"
	"encoding/json"
	jsoniter "github.com/json-iterator/go"
	"github.com/ppanyukov/azure-resource-graph-go/pkg/rg"
	"log"
	"os"
)

func main() {
	const query = `
		resources | project id
	`

	// Exec the query and get list of raw json pages
	items, err := rg.Exec[json.RawMessage](context.Background(), query, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Print out JSON
	data, err := jsoniter.MarshalIndent(items, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	_, _ = os.Stdout.Write(data)
}
