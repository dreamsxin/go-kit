package rest_test

// A fuzz target for the query string a resource route reads.
//
// The filter dialect, the sort list, the projection, the window — every one of
// them is text from a client, parsed by code that has to answer either a result
// or a classified refusal. A bare error becomes a 500, and a 500 on a malformed
// parameter is the one status that tells an operator to look at the service
// instead of at the request.
//
// So the assertions are about what an answer may be, not about which answer this
// input gets: never 500, always a code when it is refused, a page body shaped the
// way the contract says when it is a 200, and — the one that matters most — a
// projection that is honoured exactly, so no query makes a column the schema
// marks unreadable appear in a record.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/resource"
)

func FuzzListQuery(f *testing.F) {
	seeds := []struct{ filter, sort, fields, page, size string }{
		{"", "", "", "", ""},
		{`{"status":"active"}`, "-price", "id,name", "1", "2"},
		{`{"$or":[{"status":"active"},{"price":{"$gte":20}}]}`, "name", "name", "2", "1"},
		{`{"createdAt":{"$dateRange":["2026-01-01T00:00:00Z","2026-03-01T00:00:00Z"]}}`, "", "", "", ""},
		{`{"name":{"$like":"a%"}}`, "-createdAt,name", "id", "", "100"},
		{`{"$not":{"status":"archived"}}`, "", "id,price,status", "3", "1"},
		{`{"rank":{"$in":[]}}`, "", "", "", ""},
		{`{"secret":"s1"}`, "", "secret", "", ""},
		{`{"price":{"$eq":"ten"}}`, "", "", "", ""},
		{`not json`, "-,", ",", "-1", "0"},
		{`{"id":{"$eq":9007199254740993}}`, "", "", "x", "101"},
	}
	for _, seed := range seeds {
		f.Add(seed.filter, seed.sort, seed.fields, seed.page, seed.size)
	}

	// One collection for the whole run: every route under test is a read, so the
	// fixture stays the same three records and an input cannot leak into the next
	// one through stored state.
	handler := mount(f, newCollection(f))

	f.Fuzz(func(t *testing.T, filter, sort, fields, page, size string) {
		values := url.Values{}
		for _, parameter := range []struct{ name, value string }{
			{"filter", filter}, {"sort", sort}, {"fields", fields}, {"page", page}, {"size", size},
		} {
			if parameter.value != "" {
				values.Set(parameter.name, parameter.value)
			}
		}
		query := values.Encode()

		list := do(t, handler, http.MethodGet, target("/products", values), "")
		requireAnswer(t, "list", query, list)
		if list.status == http.StatusOK {
			requirePageBody(t, query, list)
			if fields != "" {
				requireProjection(t, query, fields, list)
			}
		}

		// The item route reads a projection and nothing else, so anything else
		// that was sent is a refusal rather than something quietly dropped.
		item := do(t, handler, http.MethodGet, target("/products/1", values), "")
		requireAnswer(t, "get", query, item)
		if stray := item.strayParameter(values); stray != "" {
			if item.status != http.StatusBadRequest {
				t.Fatalf("get answered %d for ?%s, which carries %q; the route reads only fields",
					item.status, query, stray)
			}
			if code := errorCode(t, item); code != "rest.parameter_not_allowed" {
				t.Fatalf("get refused ?%s with code %q, want rest.parameter_not_allowed", query, code)
			}
		}

		aggregate := url.Values{"function": {"count"}}
		if filter != "" {
			aggregate.Set("filter", filter)
		}
		answer := do(t, handler, http.MethodGet, target("/products:aggregate", aggregate), "")
		requireAnswer(t, "aggregate", aggregate.Encode(), answer)
	})
}

func FuzzQueryString(f *testing.F) {
	seeds := []string{
		``,
		`filter=%7B%22status%22%3A%22active%22%7D`,
		`filter={"status":"active"}&size=1&page=2`,
		`filter=%7B%22price%22%3A%7B%22%24gte%22%3A20%7D%7D&sort=-price`,
		`sort=price,name&size=100&page=1`,
		`fields=secret`,
		`append=owner&fields=id,name`,
		// Query strings the URL parser cannot read in full. They are the reason
		// this target exists: keeping the pairs that parsed answers a broader
		// question than the one the client thought it asked.
		`filter=%zz`,
		`filter={"status":"active"};size=10`,
		`filter=%7B%7D&%zz=1`,
		`size=1&filter={"status":"active"}&filter={"price":{"$lt":25}}`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	handler := mount(f, newCollection(f))

	f.Fuzz(func(t *testing.T, rawQuery string) {
		result := doRaw(t, handler, http.MethodGet, "/products", rawQuery)

		if _, err := url.ParseQuery(rawQuery); err != nil {
			if result.status != http.StatusBadRequest {
				t.Fatalf("?%s cannot be parsed (%v) and answered %d: %s",
					rawQuery, err, result.status, result.body)
			}
			if code := errorCode(t, result); code != "rest.query_malformed" {
				t.Fatalf("?%s cannot be parsed and was refused with code %q, want rest.query_malformed",
					rawQuery, code)
			}
			return
		}

		requireAnswer(t, "list", rawQuery, result)
		if result.status == http.StatusOK {
			requirePageBody(t, rawQuery, result)
		}
	})
}

// requireAnswer pins the classification of every outcome: a request either
// succeeds or is refused as a request, and a refusal names its reason.
//
// A 500 is the failure this checks for, and the only one: it is the status every
// unclassified error lands on, so a 500 means a parameter a client can type
// became a fault, and the body is the one place the service will not explain
// itself. A 501 or a 503 is a deliberate classification — "this repository
// expands no relations" is answered 501 on purpose — and it names its code like
// any other refusal.
func requireAnswer(t *testing.T, route, query string, result response) {
	t.Helper()
	if result.status == http.StatusInternalServerError {
		t.Fatalf("%s answered 500 for ?%s, which means a request became an unclassified fault: %s",
			route, query, result.body)
	}
	if result.status >= http.StatusBadRequest && result.status != http.StatusMethodNotAllowed {
		if code := errorCode(t, result); code == "" {
			t.Fatalf("%s answered %d without a code for ?%s: %s", route, result.status, query, result.body)
		}
	}
}

// requirePageBody checks a 200 against the page contract the package documents:
// every key present, items an array rather than null, the window inside its
// bounds, and no record carrying a column the schema marks unreadable.
func requirePageBody(t *testing.T, query string, result response) {
	t.Helper()
	var body struct {
		Items   *[]map[string]any `json:"items"`
		Total   *int              `json:"total"`
		Number  *int              `json:"page"`
		Size    *int              `json:"size"`
		HasNext *bool             `json:"has_next"`
	}
	if err := json.Unmarshal(result.body, &body); err != nil {
		t.Fatalf("list answered 200 for ?%s with a body that is not a page: %v\n%s", query, err, result.body)
	}
	switch {
	case body.Items == nil:
		t.Fatalf("list answered 200 for ?%s with items null: %s", query, result.body)
	case body.Total == nil, body.Number == nil, body.Size == nil, body.HasNext == nil:
		t.Fatalf("list answered 200 for ?%s with a missing page field: %s", query, result.body)
	}
	if *body.Number < 1 {
		t.Fatalf("list answered 200 for ?%s on page %d", query, *body.Number)
	}
	if *body.Size < 1 || *body.Size > resource.MaxPageSize {
		t.Fatalf("list answered 200 for ?%s with size %d", query, *body.Size)
	}
	if len(*body.Items) > *body.Size {
		t.Fatalf("list answered 200 for ?%s with %d items in a window of %d", query, len(*body.Items), *body.Size)
	}
	if *body.Total < len(*body.Items) {
		t.Fatalf("list answered 200 for ?%s with total %d under %d items", query, *body.Total, len(*body.Items))
	}
	for _, record := range *body.Items {
		if _, leaked := record["secret"]; leaked {
			t.Fatalf("list answered 200 for ?%s with a field the schema marks unreadable: %v", query, record)
		}
	}
}

// requireProjection checks that a projection narrows: a record carries the
// columns that were asked for and no others.
func requireProjection(t *testing.T, query, fields string, result response) {
	t.Helper()
	allowed := make(map[string]struct{})
	for _, field := range strings.Split(fields, ",") {
		allowed[strings.TrimSpace(field)] = struct{}{}
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(result.body, &body); err != nil {
		t.Fatalf("list answered 200 for ?%s with a body that is not a page: %v", query, err)
	}
	for _, record := range body.Items {
		for name := range record {
			if _, ok := allowed[name]; !ok {
				t.Fatalf("?%s asked for %q and the record also carries %q: %v", query, fields, name, record)
			}
		}
	}
}

// strayParameter names the first parameter of a request the item route does not
// read, in the order the binding refuses them.
func (r response) strayParameter(values url.Values) string {
	for _, name := range []string{"filter", "sort", "append", "page", "size"} {
		if values.Has(name) {
			return name
		}
	}
	return ""
}
