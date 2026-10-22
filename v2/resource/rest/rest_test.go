package rest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/resource"
	"github.com/dreamsxin/go-kit/v2/resource/memory"
	"github.com/dreamsxin/go-kit/v2/resource/rest"
)

// --- fixtures ---------------------------------------------------------------

func productSchema(t *testing.T) resource.Schema {
	t.Helper()
	id := resource.NewField("id", resource.TypeInt)
	name := resource.NewField("name", resource.TypeString)
	name.Writable = true
	price := resource.NewField("price", resource.TypeFloat)
	price.Writable = true
	status := resource.NewField("status", resource.TypeString)
	status.Writable = true
	created := resource.NewField("createdAt", resource.TypeTime)
	created.Writable = true
	// A writable integer, so a test can send one wider than 2^53 and see
	// whether the body decode kept it exact.
	rank := resource.NewField("rank", resource.TypeInt)
	rank.Writable = true
	// A column that exists, may be written, and must never leave the service —
	// a credential or a private note. It is the fixture's proof that writing
	// and reading are separate capabilities.
	secret := resource.NewField("secret", resource.TypeString)
	secret.Readable = false
	secret.Writable = true
	schema, err := resource.NewSchema(id, name, price, status, created, rank, secret)
	if err != nil {
		t.Fatalf("NewSchema: %v", err)
	}
	return schema
}

// newCollection seeds three products: two active, one archived.
func newCollection(t *testing.T) resource.Collection {
	t.Helper()
	schema := productSchema(t)
	store, err := memory.New(schema, "id")
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	seed := []resource.Record{
		{"name": "alpha", "price": 10.0, "status": "active", "createdAt": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "secret": "s1"},
		{"name": "beta", "price": 30.0, "status": "active", "createdAt": time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), "secret": "s2"},
		{"name": "gamma", "price": 20.0, "status": "archived", "createdAt": time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), "secret": "s3"},
	}
	for _, record := range seed {
		if _, err := store.Create(context.Background(), record, resource.WriteMask{}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	collection, err := resource.NewCollection("products", schema, store, "id")
	if err != nil {
		t.Fatalf("NewCollection: %v", err)
	}
	return collection
}

// mount registers every route on a plain mux, which is the assembly that has no
// component to lean on.
//
// It declares the surface open, because a collection with no decision is refused
// and these tests are about what the routes answer rather than about who may ask.
// A test that installs a policy uses mountWith instead: the two options
// contradict each other by design.
func mount(t *testing.T, collection resource.Collection, options ...rest.Option) http.Handler {
	t.Helper()
	open := append(append([]rest.Option(nil), options...), rest.WithOpenAccess())
	return mountWith(t, collection, open...)
}

// mountWith mounts exactly what it is given, so a test can install a policy and
// be refused by it.
func mountWith(t *testing.T, collection resource.Collection, options ...rest.Option) http.Handler {
	t.Helper()
	routes, err := rest.Routes(collection, options...)
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	mux := http.NewServeMux()
	for _, route := range routes {
		mux.Handle(route.Pattern, route.Handler())
	}
	return mux
}

// allow is a policy that grants every action with the constraints given. It is
// what a test installs when the subject under test is how the constraints are
// merged rather than how the decision is made.
func allow(params resource.Params) rest.Policy {
	return func(context.Context, rest.Action) (rest.Decision, error) {
		return rest.Allow(params), nil
	}
}

type response struct {
	status  int
	header  http.Header
	body    []byte
	decoded map[string]any
}

func do(t *testing.T, handler http.Handler, method, target, body string) response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, target, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	result := response{status: recorder.Code, header: recorder.Header(), body: recorder.Body.Bytes()}
	if len(result.body) > 0 {
		_ = json.Unmarshal(result.body, &result.decoded)
	}
	return result
}

func errorCode(t *testing.T, result response) string {
	t.Helper()
	code, _ := result.decoded["code"].(string)
	return code
}

func target(path string, values url.Values) string {
	if len(values) == 0 {
		return path
	}
	return path + "?" + values.Encode()
}

// --- the route table --------------------------------------------------------

func TestRoutesAreTheDocumentedTable(t *testing.T) {
	routes, err := rest.Routes(newCollection(t), rest.WithOpenAccess())
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	var patterns []string
	for _, route := range routes {
		patterns = append(patterns, route.Pattern)
	}
	want := []string{
		"GET /products",
		"POST /products",
		"GET /products:aggregate",
		"GET /products/{id}",
		"PATCH /products/{id}",
		"DELETE /products/{id}",
	}
	if strings.Join(patterns, "\n") != strings.Join(want, "\n") {
		t.Fatalf("patterns = %v, want %v", patterns, want)
	}
	for _, route := range routes {
		if route.Endpoint == nil || route.Decode == nil {
			t.Fatalf("route %q carries no endpoint or decoder", route.Pattern)
		}
	}
}

func TestRoutesUnderAPrefix(t *testing.T) {
	routes, err := rest.Routes(newCollection(t), rest.WithPrefix("api/v1/"), rest.WithOpenAccess())
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	if routes[0].Pattern != "GET /api/v1/products" {
		t.Fatalf("first pattern = %q, want GET /api/v1/products", routes[0].Pattern)
	}
	if routes[2].Pattern != "GET /api/v1/products:aggregate" {
		t.Fatalf("aggregate pattern = %q", routes[2].Pattern)
	}
}

func TestRoutesRefuseACompositeKey(t *testing.T) {
	schema := productSchema(t)
	store, err := memory.New(schema, "id", "name")
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	collection, err := resource.NewCollection("products", schema, store, "id", "name")
	if err != nil {
		t.Fatalf("NewCollection: %v", err)
	}
	if _, err := rest.Routes(collection); err == nil {
		t.Fatal("Routes accepted a composite key, which one path segment cannot name")
	}
}

func TestRoutesRefuseAWildcardPrefix(t *testing.T) {
	if _, err := rest.Routes(newCollection(t), rest.WithPrefix("/api/{tenant}")); err == nil {
		t.Fatal("Routes accepted a wildcard prefix")
	}
}

// --- list -------------------------------------------------------------------

func TestListReturnsTheDefaultWindow(t *testing.T) {
	result := do(t, mount(t, newCollection(t)), http.MethodGet, "/products", "")
	if result.status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", result.status, result.body)
	}
	if total := result.decoded["total"]; total != float64(3) {
		t.Fatalf("total = %v, want 3", total)
	}
	if page := result.decoded["page"]; page != float64(resource.DefaultPageNumber) {
		t.Fatalf("page = %v, want %d", page, resource.DefaultPageNumber)
	}
	items, _ := result.decoded["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	first, _ := items[0].(map[string]any)
	if _, leaked := first["secret"]; leaked {
		t.Fatal("the list carries an unreadable column")
	}
}

func TestListFiltersSortsAndProjects(t *testing.T) {
	values := url.Values{
		"filter": {`{"status":"active"}`},
		"sort":   {"-price"},
		"fields": {"name,price"},
	}
	result := do(t, mount(t, newCollection(t)), http.MethodGet, target("/products", values), "")
	if result.status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", result.status, result.body)
	}
	items, _ := result.decoded["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %d, want the two active rows", len(items))
	}
	first, _ := items[0].(map[string]any)
	if first["name"] != "beta" {
		t.Fatalf("first = %v, want beta (the most expensive active row)", first["name"])
	}
	if _, present := first["status"]; present {
		t.Fatal("projection kept a column the client did not ask for")
	}
}

func TestListRejectsWhatItCannotAnswer(t *testing.T) {
	cases := []struct {
		name   string
		values url.Values
		status int
		code   string
	}{
		{"unknown field", url.Values{"filter": {`{"nope":1}`}}, http.StatusBadRequest, "filter.unknown_field"},
		{"malformed filter", url.Values{"filter": {`{`}}, http.StatusBadRequest, "filter.malformed"},
		{"size above the cap", url.Values{"size": {"101"}}, http.StatusBadRequest, "bad_request.validation"},
		{"empty projection", url.Values{"fields": {""}}, http.StatusBadRequest, "resource.fields_empty"},
		{"empty sort entry", url.Values{"sort": {"name,"}}, http.StatusBadRequest, "rest.sort_malformed"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := do(t, mount(t, newCollection(t)), http.MethodGet, target("/products", testCase.values), "")
			if result.status != testCase.status {
				t.Fatalf("status = %d, want %d (body %s)", result.status, testCase.status, result.body)
			}
			if code := errorCode(t, result); code != testCase.code {
				t.Fatalf("code = %q, want %q (body %s)", code, testCase.code, result.body)
			}
		})
	}
}

// --- one record -------------------------------------------------------------

func TestGetProjectsAndReportsMissing(t *testing.T) {
	handler := mount(t, newCollection(t))

	found := do(t, handler, http.MethodGet, "/products/2?fields=name", "")
	if found.status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", found.status, found.body)
	}
	if found.decoded["name"] != "beta" {
		t.Fatalf("name = %v", found.decoded["name"])
	}
	if _, present := found.decoded["price"]; present {
		t.Fatal("projection kept a column the client did not ask for")
	}

	missing := do(t, handler, http.MethodGet, "/products/999", "")
	if missing.status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", missing.status, missing.body)
	}

	malformed := do(t, handler, http.MethodGet, "/products/abc", "")
	if malformed.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a key that is not an integer", malformed.status)
	}
	if code := errorCode(t, malformed); code != "resource.key_malformed" {
		t.Fatalf("code = %q, want resource.key_malformed", code)
	}
}

func TestItemRoutesRefuseListParameters(t *testing.T) {
	result := do(t, mount(t, newCollection(t)), http.MethodGet, `/products/1?filter={"status":"active"}`, "")
	if result.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", result.status)
	}
	if code := errorCode(t, result); code != "rest.parameter_not_allowed" {
		t.Fatalf("code = %q, want rest.parameter_not_allowed", code)
	}
}

// --- writes -----------------------------------------------------------------

func TestCreateAnswers201WithTheStoredRecord(t *testing.T) {
	handler := mount(t, newCollection(t))
	result := do(t, handler, http.MethodPost, "/products", `{"name":"delta","price":40,"status":"active"}`)
	if result.status != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", result.status, result.body)
	}
	if location := result.header.Get("Location"); location != "/products/4" {
		t.Fatalf("Location = %q, want /products/4", location)
	}
	if result.decoded["name"] != "delta" {
		t.Fatalf("body = %s", result.body)
	}
	if _, leaked := result.decoded["secret"]; leaked {
		t.Fatal("the create response carries an unreadable column")
	}
	if follow := do(t, handler, http.MethodGet, "/products/4", ""); follow.status != http.StatusOK {
		t.Fatalf("the created record is not readable: %d", follow.status)
	}
}

func TestCreateRefusesWhatTheSchemaAndMaskForbid(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"unknown field", `{"nickname":"d"}`, http.StatusBadRequest, "resource.unknown_field"},
		{"unwritable field", `{"id":9}`, http.StatusForbidden, "resource.field_not_writable"},
		{"wrong type", `{"price":"cheap"}`, http.StatusBadRequest, "resource.value_type_mismatch"},
		{"not an object", `[1,2]`, http.StatusBadRequest, "rest.body_not_an_object"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := do(t, mount(t, newCollection(t)), http.MethodPost, "/products", testCase.body)
			if result.status != testCase.status {
				t.Fatalf("status = %d, want %d (body %s)", result.status, testCase.status, result.body)
			}
			if code := errorCode(t, result); code != testCase.code {
				t.Fatalf("code = %q, want %q (body %s)", code, testCase.code, result.body)
			}
		})
	}
}

// TestCreateKeepsAnIntegerBeyondTwoToTheFiftyThird proves the body is not
// decoded into float64 on the way in: a rounded identifier addresses a different
// record, and the error would surface much later than the request.
func TestCreateKeepsAnIntegerBeyondTwoToTheFiftyThird(t *testing.T) {
	const exact = "9007199254740993" // 2^53 + 1, the first integer float64 cannot hold
	handler := mount(t, newCollection(t))
	created := do(t, handler, http.MethodPost, "/products", `{"name":"epsilon","rank":`+exact+`}`)
	if created.status != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", created.status, created.body)
	}
	if !strings.Contains(string(created.body), exact) {
		t.Fatalf("the response lost the integer: %s", created.body)
	}
	read := do(t, handler, http.MethodGet, "/products?filter={\"name\":\"epsilon\"}", "")
	if !strings.Contains(string(read.body), exact) {
		t.Fatalf("the stored value is not the one that was sent: %s", read.body)
	}
}

func TestUpdateChangesOnlyTheFieldsItNames(t *testing.T) {
	handler := mount(t, newCollection(t))
	result := do(t, handler, http.MethodPatch, "/products/1", `{"status":"archived"}`)
	if result.status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", result.status, result.body)
	}
	if result.decoded["status"] != "archived" {
		t.Fatalf("status = %v", result.decoded["status"])
	}
	if result.decoded["name"] != "alpha" {
		t.Fatalf("a patch that named one field changed another: %s", result.body)
	}
}

func TestDestroyAnswers204AndRemovesTheRecord(t *testing.T) {
	handler := mount(t, newCollection(t))
	result := do(t, handler, http.MethodDelete, "/products/3", "")
	if result.status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body %s)", result.status, result.body)
	}
	if len(result.body) != 0 {
		t.Fatalf("body = %q, want none", result.body)
	}
	if follow := do(t, handler, http.MethodGet, "/products/3", ""); follow.status != http.StatusNotFound {
		t.Fatalf("the deleted record still answers: %d", follow.status)
	}
}

func TestCreateRefusesABodyOverTheLimit(t *testing.T) {
	collection := newCollection(t)
	result := do(t, mount(t, collection, rest.WithMaxBodyBytes(16)), http.MethodPost, "/products",
		`{"name":"a rather long name that does not fit"}`)
	if result.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 (body %s)", result.status, result.body)
	}
	if code := errorCode(t, result); code != "request_too_large" {
		t.Fatalf("code = %q, want request_too_large", code)
	}
}

// --- aggregate --------------------------------------------------------------

func TestAggregateGroupsTheFilteredRecords(t *testing.T) {
	values := url.Values{"function": {"count"}, "group_by": {"status"}}
	result := do(t, mount(t, newCollection(t)), http.MethodGet, target("/products:aggregate", values), "")
	if result.status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", result.status, result.body)
	}
	rows, _ := result.decoded["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want one per status (body %s)", len(rows), result.body)
	}
}

func TestAggregateNeedsAFunction(t *testing.T) {
	result := do(t, mount(t, newCollection(t)), http.MethodGet, "/products:aggregate", "")
	if result.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", result.status)
	}
	if code := errorCode(t, result); code != "rest.aggregate_function_required" {
		t.Fatalf("code = %q", code)
	}
}

// --- the layer seam ---------------------------------------------------------

// TestAPolicyNarrowsAndIsNeverWidenedByTheClient is the reason the merge runs the
// policies after the client: a policy that selects active rows and a client that
// asks for archived ones must intersect to nothing, not let either win.
func TestAPolicyNarrowsAndIsNeverWidenedByTheClient(t *testing.T) {
	onlyActive := allow(resource.Params{Filter: resource.Condition("status", resource.OpEq, "active")})
	handler := mountWith(t, newCollection(t), rest.WithPolicy(onlyActive))

	everything := do(t, handler, http.MethodGet, "/products", "")
	if total := everything.decoded["total"]; total != float64(2) {
		t.Fatalf("total = %v, want the layer's two active rows", total)
	}
	contradiction := do(t, handler, http.MethodGet, target("/products", url.Values{"filter": {`{"status":"archived"}`}}), "")
	if total := contradiction.decoded["total"]; total != float64(0) {
		t.Fatalf("total = %v, want none: the client's condition cannot replace the layer's", total)
	}
}

func TestAPolicyNarrowsTheWritableSet(t *testing.T) {
	noStatus := allow(resource.Params{Writable: resource.WriteMask{Deny: []string{"status"}}})
	result := do(t, mountWith(t, newCollection(t), rest.WithPolicy(noStatus)), http.MethodPost, "/products",
		`{"name":"delta","status":"active"}`)
	if result.status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body %s)", result.status, result.body)
	}
	if code := errorCode(t, result); code != "resource.field_not_writable" {
		t.Fatalf("code = %q", code)
	}
}

// TestAPolicyOutranksTheClientOnSortAndWindow pins the part of the merge order
// that is not commutative: ordering is decided by the last authority, and the
// page size by the smallest. A default comes before the client, a policy after
// it — so a resource default can be overridden by a request while a policy
// cannot.
func TestAPolicyOutranksTheClientOnSortAndWindow(t *testing.T) {
	collection := newCollection(t)

	byName := allow(resource.Params{Sort: []resource.Sort{resource.Asc("name")}})
	oneAtATime := allow(resource.Params{Window: resource.Window{Size: 1}})
	handler := mountWith(t, collection, rest.WithPolicy(byName, oneAtATime))

	result := do(t, handler, http.MethodGet, "/products?sort=-price&size=100", "")
	if result.status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", result.status, result.body)
	}
	if size := result.decoded["size"]; size != float64(1) {
		t.Fatalf("size = %v, want the layer's 1, not the client's 100", size)
	}
	items, _ := result.decoded["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	first, _ := items[0].(map[string]any)
	if first["name"] != "alpha" {
		t.Fatalf("first = %v, want alpha: the layer's order decides, not the client's", first["name"])
	}

	defaulted := mount(t, collection, rest.WithDefaults(resource.Params{Sort: []resource.Sort{resource.Asc("name")}}))
	overridden := do(t, defaulted, http.MethodGet, "/products?sort=-price", "")
	items, _ = overridden.decoded["items"].([]any)
	first, _ = items[0].(map[string]any)
	if first["name"] != "beta" {
		t.Fatalf("first = %v, want beta: a request overrides a resource default", first["name"])
	}
}

// TestAProjectionPolicyHoldsOnEveryAnswer pins the parts of a column restriction
// that a list projection does not cover: the record a write returns, and the
// figure an aggregate computes. Summing a column or grouping by it reveals what
// listing it would, so a restricted column is refused there too.
func TestAProjectionLayerHoldsOnEveryAnswer(t *testing.T) {
	twoColumns := allow(resource.Params{Fields: []string{"id", "name"}})
	handler := mountWith(t, newCollection(t), rest.WithPolicy(twoColumns))

	listed := do(t, handler, http.MethodGet, "/products", "")
	items, _ := listed.decoded["items"].([]any)
	first, _ := items[0].(map[string]any)
	if len(first) != 2 {
		t.Fatalf("listed record = %v, want only the two allowed columns", first)
	}

	created := do(t, handler, http.MethodPost, "/products", `{"name":"delta","price":40}`)
	if created.status != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", created.status, created.body)
	}
	if _, present := created.decoded["price"]; present {
		t.Fatalf("the create answer carries a column the projection excludes: %s", created.body)
	}
	if created.header.Get("Location") == "" {
		t.Fatal("the create answer lost the record's address")
	}

	// The identifier may itself be hidden from the body. The address is read
	// before the projection, so hiding it does not make the record
	// unaddressable.
	nameOnly := allow(resource.Params{Fields: []string{"name"}})
	bare := do(t, mountWith(t, newCollection(t), rest.WithPolicy(nameOnly)), http.MethodPost, "/products", `{"name":"epsilon"}`)
	if bare.status != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", bare.status, bare.body)
	}
	if _, present := bare.decoded["id"]; present {
		t.Fatalf("the answer carries the identifier the projection hid: %s", bare.body)
	}
	if bare.header.Get("Location") != "/products/4" {
		t.Fatalf("Location = %q, want the address derived before projection", bare.header.Get("Location"))
	}

	aggregate := do(t, handler, http.MethodGet, "/products:aggregate?function=count&group_by=status", "")
	if aggregate.status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a group the projection excludes (body %s)", aggregate.status, aggregate.body)
	}
	if code := errorCode(t, aggregate); code != "rest.aggregate_field_not_allowed" {
		t.Fatalf("code = %q", code)
	}
}

func TestAPolicySeesWhichActionItIsAnswering(t *testing.T) {
	var seen []rest.Action
	recorder := func(_ context.Context, action rest.Action) (rest.Decision, error) {
		seen = append(seen, action)
		return rest.Allow(resource.Params{}), nil
	}
	handler := mountWith(t, newCollection(t), rest.WithPolicy(recorder))
	do(t, handler, http.MethodGet, "/products", "")
	do(t, handler, http.MethodDelete, "/products/1", "")
	want := []rest.Action{rest.ActionList, rest.ActionDestroy}
	if len(seen) != len(want) || seen[0] != want[0] || seen[1] != want[1] {
		t.Fatalf("actions = %v, want %v", seen, want)
	}
}

// --- response shape ---------------------------------------------------------

func TestTheWireShapeMatchesTheTransport(t *testing.T) {
	result := do(t, mount(t, newCollection(t)), http.MethodGet, "/products?size=1", "")
	var page struct {
		Items   []map[string]any `json:"items"`
		Total   int              `json:"total"`
		Page    int              `json:"page"`
		Size    int              `json:"size"`
		HasNext bool             `json:"has_next"`
	}
	if err := json.Unmarshal(result.body, &page); err != nil {
		t.Fatalf("body %s is not a page: %v", result.body, err)
	}
	if page.Total != 3 || page.Page != 1 || page.Size != 1 || !page.HasNext {
		t.Fatalf("page = %+v", page)
	}
	if !bytes.Contains(result.body, []byte(`"has_next":true`)) {
		t.Fatalf("body %s does not use the transport's field names", result.body)
	}
}
