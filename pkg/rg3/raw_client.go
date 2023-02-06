package rg3

import (
	"bytes"
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/buger/jsonparser"
	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"strconv"
	"strings"
	"time"
)

// TODO: do it properly
var token, tokenErr = func() (azcore.AccessToken, error) {
	start := time.Now()
	defer func() {
		log.Printf("Auth Token acquire time: %s", time.Since(start))
	}()

	const audience = "https://management.core.windows.net//.default"
	var res azcore.AccessToken

	ctx := context.Background()

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return res, err
	}

	return cred.GetToken(ctx, struct{ Scopes []string }{Scopes: []string{audience}})
}()

type RawQueryClient struct {
	// TODO: this is temporary only
	DumpRequests       bool
	DumpResponses      bool
	DumpResponseBodies bool
}

func NewRawQueryClient() *RawQueryClient {
	return &RawQueryClient{}
}

func (client *RawQueryClient) Do(queryRequest *RawQueryRequest) (*RawQueryResultsPager, error) {
	return &RawQueryResultsPager{
		client:       client,
		queryRequest: queryRequest,
		result:       nil,
	}, nil
}

func (client *RawQueryClient) createPageRequest(queryRequest *RawQueryRequest, skipToken string) (*http.Request, error) {
	// TODO: temporary, do this better
	if tokenErr != nil {
		return nil, tokenErr
	}

	// TODO: do we need to support other clouds etc?
	const url = "https://management.azure.com/providers/Microsoft.ResourceGraph/resources?api-version=2021-06-01-preview"

	type queryRequestOptionsRaw struct {
		AllowPartialScopes bool    `json:"allowPartialScopes,omitempty"`
		ResultFormat       string  `json:"resultFormat,omitempty"`
		Top                *int32  `json:"$top,omitempty"`
		Skip               *int32  `json:"$skip,omitempty"`
		SkipToken          *string `json:"$skipToken,omitempty"`
	}

	type queryRequestRaw struct {
		Query            string                  `json:"query,omitempty"`
		ManagementGroups []string                `json:"managementGroups,omitempty"`
		Subscriptions    []string                `json:"subscriptions,omitempty"`
		Options          *queryRequestOptionsRaw `json:"options,omitempty"`
	}

	q := queryRequestRaw{
		Query:            queryRequest.Query,
		ManagementGroups: queryRequest.ManagementGroups,
		Subscriptions:    queryRequest.Subscriptions,
		Options: &queryRequestOptionsRaw{
			AllowPartialScopes: queryRequest.AllowPartialScopes,
			ResultFormat:       "objectArray",
			Top:                nil,
			Skip:               nil,
			SkipToken:          nil,
		},
	}

	if skipToken != "" {
		q.Options.SkipToken = &skipToken
	}

	top := int32(queryRequest.Top)
	if queryRequest.Top > 0 {
		q.Options.Top = &top
	}

	skip := int32(queryRequest.Skip)
	if queryRequest.Skip > 0 {
		q.Options.Skip = &skip
	}

	reqBody, err := jsoniter.Marshal(&q)
	if err != nil {
		return nil, errors.WithMessage(err, "cannot marshal query body")
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, errors.WithMessage(err, "cannot create HTTP request")
	}

	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewBuffer(reqBody)), nil
	}

	req.Header.Set("Authorization", "Bearer "+token.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	if client.DumpRequests {
		// TODO: hide authorization header?
		d, err := httputil.DumpRequestOut(req, true)
		if err != nil {
			return nil, err
		}
		os.Stdout.Write(d)
		os.Stdout.Write([]byte("\n"))
	}

	return req, nil
}

func (client *RawQueryClient) doPageRequest(req *http.Request) (*http.Response, error) {
	// Handle http.StatusTooManyRequests etc
	// When we run queries in very quick sequence, we may get HTTP 429 with these headers and body
	//
	//     HTTP/2.0 429 Too Many Requests
	//     Content-Length: 421
	//     Cache-Control: no-cache
	//     Content-Type: application/json; charset=utf-8
	//     Date: Fri, 03 Feb 2023 17: 58: 40 GMT
	//     Expires: -1
	//     Pragma: no-cache
	//     Retry-After: 2
	//     Server: Kestrel
	//     Strict-Transport-Security: max-age=31536000; includeSubDomains
	//     X-Content-Type-Options: nosniff
	//     X-Ms-Correlation-Request-Id: 8e4201ec-a5ea-4b6a-be58-75c610acbbe8
	//     X-Ms-Ratelimit-Remaining-Tenant-Reads: 11984
	//     X-Ms-Ratelimit-Remaining-Tenant-Resource-Requests: 0
	//     X-Ms-Request-Id: 8e4201ec-a5ea-4b6a-be58-75c610acbbe8
	//     X-Ms-Resource-Graph-Request-Duration: 0: 00: 00: 00.0138132
	//     X-Ms-Routing-Request-Id: UKSOUTH: 20230203T175841Z: 8e4201ec-a5ea-4b6a-be58-75c610acbbe8
	//     X-Ms-User-Quota-Remaining: 0
	//     X-Ms-User-Quota-Resets-After: 00: 00: 05
	//
	//
	//     {
	//         "error": {
	//             "code": "RateLimiting",
	//             "message": "Please provide below info when asking for support: timestamp = 2023-02-03T17:58:41.1380553Z, correlationId = 8e4201ec-a5ea-4b6a-be58-75c610acbbe8.",
	//             "details": [
	//                 {
	//                     "code": "RateLimiting",
	//                     "message": "Client application has been throttled and should not attempt to repeat the request until an amount of time has elapsed. Please see https://aka.ms/resourcegraph-throttling for help."
	//                 }
	//             ]
	//         }
	//     }
	for i := 0; ; i++ {
		// reset the body on retries otherwise it doesn't get sent
		if i > 0 {
			// TODO: we kind of assume that we set this func, when we created the request.
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = body
		}

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return res, err
		}

		if res.StatusCode == http.StatusOK {
			return res, nil
		}

		// Give up on any other error
		if res.StatusCode != http.StatusTooManyRequests {
			return res, err
		}

		// We are handling StatusTooManyRequests from here on.
		//
		// Read and close response body, don't care about errors
		_, _ = io.ReadAll(res.Body)
		_ = res.Body.Close()

		// Get the time to sleep before retry. Use arbitrary default value in case we didn't get it in the headers.
		retryAfter := func() time.Duration {
			retryDefault := 2 * time.Second

			retryAfterSecStr := res.Header.Get("Retry-After")
			if retryAfterSecStr == "" {
				return retryDefault
			}

			i, err := strconv.Atoi(retryAfterSecStr)
			if err != nil {
				return retryDefault
			}

			return time.Duration(i) * time.Second
		}()

		log.Printf("XXX: http.StatusTooManyRequests. Retry-After: %v\n", retryAfter)
		time.Sleep(retryAfter)
	}
}

func (client *RawQueryClient) getPage(queryRequest *RawQueryRequest, skipToken string) (*RawQueryResultPage, error) {
	req, err := client.createPageRequest(queryRequest, skipToken)
	if err != nil {
		return nil, err
	}

	res, err := client.doPageRequest(req)
	if err != nil {
		// ignore errors
		// TODO: can do body reads with sync.Pool?
		_, _ = io.ReadAll(res.Body)
		_ = res.Body.Close()
		return nil, err
	}

	if client.DumpResponses {
		d, _ := httputil.DumpResponse(res, client.DumpResponseBodies)
		os.Stdout.Write(d)
		os.Stdout.Write([]byte("\n"))
		os.Stdout.Write([]byte("\n"))
	}

	// TODO: can do body reads with sync.Pool?
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.WithMessage(err, "cannot read response body")
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.Errorf("cannot execute query: %v", string(resBody))
	}

	return newQueryResultPageRaw2(client, queryRequest, req, res, resBody)
}

type RawQueryRequest struct {
	// REQUIRED; The resources query.
	Query string
	// Azure management groups against which to execute the query. Example: [ 'mg1', 'mg2' ]
	ManagementGroups []string
	// Azure subscriptions against which to execute the query.
	Subscriptions []string
	// Only applicable for tenant and management group level queries to decide whether to allow partial scopes for result in case
	// the number of subscriptions exceed allowed limits.
	AllowPartialScopes bool
	// The number of rows to skip from the beginning of the results. Overrides the next page offset when $skipToken property is
	// present.
	Skip int
	// The maximum number of rows that the query should return. Overrides the page size when $skipToken property is present.
	Top int
}

type RawQueryResultsPager struct {
	client       *RawQueryClient
	queryRequest *RawQueryRequest
	result       *RawQueryResultPage
}

func (pager *RawQueryResultsPager) HasNext() bool {
	result := pager.result == nil || pager.result.HasNext()
	return result
}

func (pager *RawQueryResultsPager) GetNext() (*RawQueryResultPage, error) {
	var nextPage *RawQueryResultPage
	var err error

	if pager.result == nil {
		nextPage, err = pager.client.getPage(pager.queryRequest, "")
	} else {
		nextPage, err = pager.result.GetNext()
	}

	pager.result = nextPage
	return nextPage, err
}

type RawQueryResultPage struct {
	client       *RawQueryClient
	queryRequest *RawQueryRequest
	request      *http.Request
	response     *http.Response
	responseBody []byte

	// Number of total records matching the query.
	totalRecords int
	// Number of records returned in the current response. In the case of paging, this is the number of records in the current page.
	count int
	// Raw results as []byte for later unmarshalling into whatever types.
	data []byte
	// Indicates whether the query results are truncated.
	resultTruncated string
	// When present, the value can be passed to a subsequent query call (together with the same query and scopes used in the current
	// request) to retrieve the next page of data.
	skipToken string

	//// These come from the response headers, see:
	//// https://learn.microsoft.com/en-us/azure/governance/resource-graph/concepts/guidance-for-throttled-requests#understand-throttling-headers
	//userQuotaRemaining         int
	//userQuotaResetsAfter       time.Duration
	//tenantSubscriptionLimitHit bool
	//queryRequestDuration       string
}

func newQueryResultPageRaw2(client *RawQueryClient, queryRequest *RawQueryRequest, req *http.Request, res *http.Response, resBody []byte) (*RawQueryResultPage, error) {
	result := RawQueryResultPage{
		client:       client,
		queryRequest: queryRequest,
		request:      req,
		response:     res,
		responseBody: resBody,

		// The rest will be set further down
	}

	// This is the response body. Order of keys seems to be stable here.
	// So get them from response in this order.
	// {
	//	"totalRecords":3456,
	//	"count":1,
	//	"data":[{"foo":"bar"}],
	//	"facets":[],
	//	"resultTruncated":"false",
	//	"$skipToken":"some value"
	// }
	paths := [][]string{
		[]string{"totalRecords"},
		[]string{"count"},
		[]string{"data"},
		[]string{"resultTruncated"},
		[]string{"$skipToken"},
	}

	// keep last parse error
	var parseError error
	jsonparser.EachKey(resBody, func(idx int, value []byte, vt jsonparser.ValueType, err error) {
		if err != nil {
			parseError = err
			return
		}

		switch idx {
		case 0:
			if vt != jsonparser.Number {
				parseError = errors.New("query response field totalRecords is not a number")
				return
			}
			s := string(value)
			n, err := strconv.Atoi(s)
			if err != nil {
				parseError = errors.Errorf("cannot convert response field totalRecords to int, value is: %q", s)
				return
			}
			result.totalRecords = n
		case 1:
			if vt != jsonparser.Number {
				parseError = errors.New("query response field count is not a number")
				return
			}
			s := string(value)
			n, err := strconv.Atoi(s)
			if err != nil {
				parseError = errors.Errorf("cannot convert response field count to int, value is: %q", s)
				return
			}
			result.count = n
		case 2:
			result.data = value
		case 3:
			result.resultTruncated = string(value)
		case 4:
			result.skipToken = string(value)
		}
	}, paths...)

	return &result, parseError
}

func (page *RawQueryResultPage) HasNext() bool {
	return page.skipToken != ""
}

func (page *RawQueryResultPage) GetNext() (*RawQueryResultPage, error) {
	if page.skipToken == "" {
		return nil, errors.New("azure graph query pager: no more pages")
	}

	return page.client.getPage(page.queryRequest, page.skipToken)
}

func (page *RawQueryResultPage) Data() []byte {
	return page.data
}

func (page *RawQueryResultPage) TotalRecords() int {
	return page.totalRecords
}

func (page *RawQueryResultPage) Count() int {
	return page.count
}

func (page *RawQueryResultPage) IsTruncated() bool {
	return strings.EqualFold(page.resultTruncated, "true")
}

// TODO: do we need these methods public?

func (page *RawQueryResultPage) UserQuotaRemaining() int {
	const defaultVal = 15

	userQuotaRemaining := page.response.Header.Get("x-ms-user-quota-remaining")
	if userQuotaRemaining != "" {
		i, err := strconv.Atoi(userQuotaRemaining)
		if err != nil {
			return defaultVal
		}
		return i
	} else {
		// set to some "default value"
		return defaultVal
	}
}

func (page *RawQueryResultPage) UserQuotaResetsAfter() time.Duration {
	const defaultVal = 5 * time.Second

	userQuotaResetsAfter := page.response.Header.Get("x-ms-user-quota-resets-after")
	if userQuotaResetsAfter != "" {
		t, err := time.Parse("15:04:05", userQuotaResetsAfter)
		if err != nil {
			return defaultVal
		}

		// time.Time{} doesn't work, see time.Parse
		empty := time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)
		return t.Sub(empty)
	} else {
		return defaultVal
	}
}

func (page *RawQueryResultPage) IsTenantSubscriptionLimitHit() bool {
	tenantSubscriptionLimitHit := page.response.Header.Get("x-ms-tenant-subscription-limit-hit")
	return strings.EqualFold(tenantSubscriptionLimitHit, "true")
}

func (page *RawQueryResultPage) QueryRequestDuration() string {
	queryRequestDuration := page.response.Header.Get("x-ms-resource-graph-request-duration")
	return queryRequestDuration
}
