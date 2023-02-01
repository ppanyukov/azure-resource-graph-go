package armresourcegraph2

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	jsoniter "github.com/json-iterator/go"
)

// This is the customisation of the original Azure SDK package using generics

// ResourcesAll2 is a convenience method which executes a query and returns all data unmarshalled into the specified type.
func ResourcesAll2[T any](client *Client, ctx context.Context, query QueryRequest) ([]T, error) {
	var result []T

	pager := Resources2[T](client, ctx, query)
	for pager.HasNext() {
		page, err := pager.Get()
		if err != nil {
			return result, err
		}

		result = append(result, page...)
	}

	return result, nil
}

// Resources2 executes a query and returns data unmarshalled into the specified type.
func Resources2[T any](client *Client, ctx context.Context, query QueryRequest) *QueryResultPager2[T] {
	return &QueryResultPager2[T]{
		client:   client,
		ctx:      ctx,
		query:    query,
		options:  nil,
		response: nil,
	}
}

// QueryResultPager2 iterates over the query result pages.
type QueryResultPager2[T any] struct {
	client   *Client
	ctx      context.Context
	query    QueryRequest
	options  *ClientResourcesOptions
	response *queryResponse2[T]
}

// HasNext tells if there is next page.
func (q *QueryResultPager2[T]) HasNext() bool {
	if q.response == nil {
		return true
	}

	return q.response.SkipToken != nil && *q.response.SkipToken != ""
}

// Get returns the data for the current page and advances to the next page.
func (q *QueryResultPager2[T]) Get() ([]T, error) {
	log.Printf("QueryResultPager2: getting next page")
	// This is broadly a copy of Client.Resources2 with modifications
	req, err := q.client.resourcesCreateRequest(q.ctx, q.query, q.options)
	if err != nil {
		return nil, err
	}
	resp, err := q.client.pl.Do(req)
	if err != nil {
		return nil, err
	}
	if !runtime.HasStatusCode(resp, http.StatusOK) {
		return nil, runtime.NewResponseError(resp)
	}

	result, err := q.unmarshalJson(resp)
	if err != nil {
		return nil, err
	}

	q.response = result
	if q.query.Options == nil {
		q.query.Options = &QueryRequestOptions{}
	}
	q.query.Options.SkipToken = result.SkipToken

	return result.Data, nil
}

// This is copied and adjusted from the Azure SDK code.
func (q *QueryResultPager2[T]) unmarshalJson(resp *http.Response) (*queryResponse2[T], error) {
	// UnmarshalAsJSON calls json.Unmarshal() to unmarshal the received payload into the value pointed to by v.
	payload, err := runtime.Payload(resp)
	if err != nil {
		return nil, err
	}

	// TODO: verify early exit is correct
	if len(payload) == 0 {
		return nil, nil
	}

	var result queryResponse2[T]
	trimmed := bytes.TrimPrefix(payload, []byte("\xef\xbb\xbf"))
	err = jsoniter.Unmarshal(trimmed, &result)
	if err != nil {
		err = fmt.Errorf("unmarshalling type %T: %s", result, err)
		return &result, err
	}

	return &result, nil
}

// queryResponse2 is the response returned by the query for a single page,
// it allows us to unmarshal data into specific type.
type queryResponse2[T any] struct {
	// REQUIRED; Number of records returned in the current response. In the case of paging, this is the number of records in the
	// current page.
	//Count *int64 `json:"count,omitempty"`

	// REQUIRED; Query output in JObject array or Table format.
	Data []T `json:"data,omitempty"`

	// REQUIRED; Indicates whether the query results are truncated.
	//ResultTruncated *ResultTruncated `json:"resultTruncated,omitempty"`

	// REQUIRED; Number of total records matching the query.
	//TotalRecords *int64 `json:"totalRecords,omitempty"`

	// Query facets.
	//Facets []FacetClassification `json:"facets,omitempty"`

	// When present, the value can be passed to a subsequent query call (together with the same query and scopes used in the current
	// request) to retrieve the next page of data.
	SkipToken *string `json:"$skipToken,omitempty"`
}
