package armresourcegraph2

import (
	"bytes"
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"log"
	"net/http"
	"reflect"
)

// This is the customisation of the original Azure SDK package
// Using non-generic types and interfaces.

// ResourcesAll3 is a convenience method which executes a query and returns all data unmarshalled into the specified type.
func ResourcesAll3(client *Client, ctx context.Context, query QueryRequest, out any) error {
	// out must be a non-null pointer to a slice
	rv := reflect.ValueOf(out)
	if rv.Kind() != reflect.Pointer {
		return errors.Errorf("the out parameter of type '%v' must be a pointer", rv.Type())
	}

	if rv.IsNil() {
		return errors.Errorf("the out parameter of type '%v' must not be null", rv.Type())
	}

	if rv.Elem().Kind() != reflect.Slice {
		return errors.Errorf("the type of out parameter '%v' must be a pointer to a slice", rv.Type())
	}

	// The actual slice
	elem1 := rv.Elem()

	// Will accumulate results from the pager here.
	result := reflect.MakeSlice(elem1.Type(), 0, 0)
	pager := Resources3(client, ctx, query)
	for pager.HasNext() {
		// Not sure why it doesn't unmarshal into correct types when we use
		// slice created with reflection. So using the originally provided slice
		// for unmarshalling then copying from it to the result.
		err := pager.Get(out)
		if err != nil {
			return err
		}

		result = reflect.AppendSlice(result, elem1)
	}

	elem1.Set(result)
	return nil
}

// Resources3 executes a query and returns data unmarshalled into the specified type.
func Resources3(client *Client, ctx context.Context, query QueryRequest) *QueryResultPager3 {
	return &QueryResultPager3{
		client:   client,
		ctx:      ctx,
		query:    query,
		options:  nil,
		response: nil,
	}
}

// QueryResultPager3 iterates over the query result pages.
type QueryResultPager3 struct {
	client   *Client
	ctx      context.Context
	query    QueryRequest
	options  *ClientResourcesOptions
	response *queryResponse3
}

// HasNext tells if there is next page.
func (q *QueryResultPager3) HasNext() bool {
	if q.response == nil {
		return true
	}

	return q.response.SkipToken != nil && *q.response.SkipToken != ""
}

// Get returns the data for the current page and advances to the next page.
func (q *QueryResultPager3) Get(out any) error {
	log.Printf("QueryResultPager2: getting next page")
	// This is broadly a copy of Client.Resources2 with modifications
	req, err := q.client.resourcesCreateRequest(q.ctx, q.query, q.options)
	if err != nil {
		return err
	}
	resp, err := q.client.pl.Do(req)
	if err != nil {
		return err
	}
	if !runtime.HasStatusCode(resp, http.StatusOK) {
		return runtime.NewResponseError(resp)
	}

	queryResult, err := q.unmarshalQueryResponse(resp)
	if err != nil {
		return err
	}

	err = jsoniter.Unmarshal(queryResult.Data, out)
	if err != nil {
		return err
	}

	q.response = queryResult
	if q.query.Options == nil {
		q.query.Options = &QueryRequestOptions{}
	}
	q.query.Options.SkipToken = queryResult.SkipToken

	return nil
}

// This is copied and adjusted from the Azure SDK code.
func (q *QueryResultPager3) unmarshalQueryResponse(resp *http.Response) (*queryResponse3, error) {
	// UnmarshalAsJSON calls json.Unmarshal() to unmarshal the received payload into the value pointed to by v.
	payload, err := runtime.Payload(resp)
	if err != nil {
		return nil, err
	}

	// TODO: verify early exit is correct
	if len(payload) == 0 {
		return nil, nil
	}

	var result queryResponse3
	trimmed := bytes.TrimPrefix(payload, []byte("\xef\xbb\xbf"))
	err = jsoniter.Unmarshal(trimmed, &result)
	if err != nil {
		err = fmt.Errorf("unmarshalling type %T: %s", result, err)
		return nil, err
	}

	return &result, nil
}

// queryResponse3 is the response returned by the query for a single page,
// it allows us to unmarshal data into specific type.
type queryResponse3 struct {
	// REQUIRED; Number of records returned in the current response. In the case of paging, this is the number of records in the
	// current page.
	//Count *int64 `json:"count,omitempty"`

	// REQUIRED; Query output in JObject array or Table format.
	Data jsoniter.RawMessage `json:"data,omitempty"`

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
