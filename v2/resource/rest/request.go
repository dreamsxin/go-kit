package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/resource"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// request is what every decoder produces and every handler consumes. One type
// for six routes, because a route reads a subset of it: a list reads params, a
// create reads a record, an item route reads a key.
type request struct {
	key       resource.Key
	params    resource.Params
	record    resource.Record
	aggregate resource.AggregateQuery
}

// The query parameters a resource route recognises. A route refuses the ones it
// does not read rather than ignoring them: a filter that was dropped answers a
// broader question, and an answer to a broader question is indistinguishable
// from an authorization failure at the call site.
var recognisedParams = []string{"filter", "sort", "fields", "append", "page", "size"}

func (b *binding) decodeList(ctx context.Context, r *http.Request) (any, error) {
	params, err := b.decide(ctx, r, ActionList)
	if err != nil {
		return nil, err
	}
	return &request{params: params}, nil
}

func (b *binding) decodeGet(ctx context.Context, r *http.Request) (any, error) {
	if err := rejectParams(r.URL.Query(), ActionGet, "fields"); err != nil {
		return nil, err
	}
	params, err := b.decide(ctx, r, ActionGet)
	if err != nil {
		return nil, err
	}
	key, err := b.collection.KeyFromText(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	return &request{key: key, params: params}, nil
}

func (b *binding) decodeCreate(ctx context.Context, r *http.Request) (any, error) {
	return b.decodeWrite(ctx, r, ActionCreate)
}

func (b *binding) decodeUpdate(ctx context.Context, r *http.Request) (any, error) {
	return b.decodeWrite(ctx, r, ActionUpdate)
}

// decodeWrite reads a create or an update: the body as a record, the layers as
// the mask, then the mask applied before storage is touched.
func (b *binding) decodeWrite(ctx context.Context, r *http.Request, action Action) (any, error) {
	if err := rejectParams(r.URL.Query(), action); err != nil {
		return nil, err
	}
	params, err := b.decide(ctx, r, action)
	if err != nil {
		return nil, err
	}
	record, err := b.decodeRecord(r)
	if err != nil {
		return nil, err
	}
	// Checked here as well as in the repository. The repository owns the rule,
	// but the answer is a 403 that names the field, and an adapter that forgot
	// to apply the mask would turn it into a write.
	if err := params.Writable.Check(record); err != nil {
		return nil, err
	}
	request := &request{params: params, record: record}
	if action == ActionCreate {
		if err := b.admits(record, params.Filter); err != nil {
			return nil, err
		}
	}
	if action == ActionUpdate {
		key, err := b.collection.KeyFromText(r.PathValue("id"))
		if err != nil {
			return nil, err
		}
		request.key = key
	}
	return request, nil
}

// admits reports whether a record a caller is storing satisfies the condition its
// policies stated.
//
// A condition on a create is a condition on the row being created: "you may add
// rows to tenant 7" is the same condition that lets you read tenant 7, evaluated
// against what is about to be written. Without this the constraint would only
// select reads, and a caller could store a row it would never be allowed to see.
//
// A field the record does not carry makes the condition unknown, which refuses.
// That is the honest answer rather than a bug: a policy that scopes a create has
// to scope it on a field the create states, because a column a database default
// fills is a column the policy cannot check before the write.
func (b *binding) admits(record resource.Record, condition resource.Filter) error {
	if condition.IsZero() {
		return nil
	}
	admitted, err := resource.Match(condition, record, b.collection.Schema)
	if err != nil {
		return err
	}
	if admitted {
		return nil
	}
	return apperror.PermissionDenied("rest.constraint_unsatisfied",
		"the record this request stores does not satisfy the condition its policy states, and a row that is not readable through this surface is not writable through it")
}

func (b *binding) decodeDestroy(ctx context.Context, r *http.Request) (any, error) {
	if err := rejectParams(r.URL.Query(), ActionDestroy); err != nil {
		return nil, err
	}
	// The params are not read by the operation, but the decision is: the
	// condition a policy grants a delete is what selects the record, and a
	// delete that ignored it would remove a row the policy never reached.
	params, err := b.decide(ctx, r, ActionDestroy)
	if err != nil {
		return nil, err
	}
	key, err := b.collection.KeyFromText(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	return &request{key: key, params: params}, nil
}

func (b *binding) decodeAggregate(ctx context.Context, r *http.Request) (any, error) {
	if err := rejectParams(r.URL.Query(), ActionAggregate, "filter"); err != nil {
		return nil, err
	}
	params, err := b.decide(ctx, r, ActionAggregate)
	if err != nil {
		return nil, err
	}
	values := r.URL.Query()
	function := resource.AggregateFunction(strings.TrimSpace(values.Get("function")))
	if function == "" {
		return nil, apperror.InvalidArgument("rest.aggregate_function_required",
			"an aggregate request needs the function parameter: count, sum, avg, min or max")
	}
	query := resource.AggregateQuery{
		Function: function,
		Field:    strings.TrimSpace(values.Get("field")),
		Filter:   params.Filter,
		GroupBy:  splitList(values.Get("group_by")),
	}
	if err := query.Validate(b.collection.Schema); err != nil {
		return nil, err
	}
	// A projection narrows what a caller may see, and an aggregate answers
	// questions about values: summing a column or grouping by it reveals what
	// listing it would. The repository applies a projection to a list, but an
	// aggregate result carries figures rather than records, so the restriction
	// has to be applied to the query here.
	if err := restrictAggregateFields(query, params.Fields); err != nil {
		return nil, err
	}
	return &request{params: params, aggregate: query}, nil
}

// restrictAggregateFields refuses a figure computed over a column the merged
// projection does not allow.
func restrictAggregateFields(query resource.AggregateQuery, allowed []string) error {
	if allowed == nil {
		return nil
	}
	if query.Field != "" && !containsString(allowed, query.Field) {
		return apperror.PermissionDenied("rest.aggregate_field_not_allowed",
			fmt.Sprintf("field %q may not be aggregated in this request", query.Field))
	}
	for _, group := range query.GroupBy {
		if !containsString(allowed, group) {
			return apperror.PermissionDenied("rest.aggregate_field_not_allowed",
				fmt.Sprintf("field %q may not be grouped in this request", group))
		}
	}
	return nil
}

// readParams assembles the layers a request is made of, in the only order that
// narrows, and refuses the ones no policy granted. It lives in policy.go with the
// decision type it exists to evaluate.

// clientParams reads what the request itself asks for.
func clientParams(r *http.Request) (resource.Params, error) {
	values := r.URL.Query()
	filter, err := parseFilterParams(values)
	if err != nil {
		return resource.Params{}, err
	}
	order, err := parseSort(values.Get("sort"))
	if err != nil {
		return resource.Params{}, err
	}
	fields, err := parseFieldList(values, "fields")
	if err != nil {
		return resource.Params{}, err
	}
	appends, err := parseFieldList(values, "append")
	if err != nil {
		return resource.Params{}, err
	}
	window, err := parseWindow(r, values)
	if err != nil {
		return resource.Params{}, err
	}
	return resource.Params{
		Filter:  filter,
		Sort:    order,
		Fields:  fields,
		Appends: appends,
		Window:  window,
	}, nil
}

// parseFilterParams reads every filter parameter. Several are a conjunction,
// which is the only combination the dialect has a spelling for at the top level
// and the only one a client may add to a policy's condition.
func parseFilterParams(values url.Values) (resource.Filter, error) {
	documents := values["filter"]
	if len(documents) == 0 {
		return resource.Filter{}, nil
	}
	filters := make([]resource.Filter, 0, len(documents))
	for _, document := range documents {
		filter, err := resource.ParseFilter(document)
		if err != nil {
			return resource.Filter{}, err
		}
		filters = append(filters, filter)
	}
	return resource.And(filters...), nil
}

// parseSort reads "sort=-createdAt,name": a leading minus reverses the order,
// a leading plus is accepted for symmetry, and an empty entry is a typo rather
// than a field with no name.
func parseSort(raw string) ([]resource.Sort, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	order := make([]resource.Sort, 0, len(parts))
	for _, part := range parts {
		field := strings.TrimSpace(part)
		descending := false
		if strings.HasPrefix(field, "-") || strings.HasPrefix(field, "+") {
			descending = field[0] == '-'
			field = field[1:]
		}
		if field == "" {
			return nil, apperror.InvalidArgument("rest.sort_malformed",
				"sort names an empty field; write fields separated by commas, with - for descending")
		}
		order = append(order, resource.Sort{Field: field, Descending: descending})
	}
	return order, nil
}

// parseFieldList reads a comma-separated parameter such as fields or append.
//
// The absent and the empty spellings stay distinct: absent is nil ("no
// opinion"), an empty value is an empty list ("none"). For a projection the
// second is refused by validation, and answering it with every column would be
// the widening this package exists to prevent.
func parseFieldList(values url.Values, name string) ([]string, error) {
	if !values.Has(name) {
		return nil, nil
	}
	raw := values.Get(name)
	if raw == "" {
		return []string{}, nil
	}
	parts := strings.Split(raw, ",")
	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		field := strings.TrimSpace(part)
		if field == "" {
			return nil, apperror.InvalidArgument("rest.list_malformed",
				fmt.Sprintf("%s names an empty field; write field names separated by commas", name))
		}
		fields = append(fields, field)
	}
	return fields, nil
}

// splitList reads a list parameter where an empty value simply means no entries.
func splitList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		if field := strings.TrimSpace(part); field != "" {
			fields = append(fields, field)
		}
	}
	return fields
}

// parseWindow reads page and size through the transport's own parser, so a
// resource request and a generated one reject the same value with the same
// message. Only the parameters that were actually sent are applied, so a
// resource that sets a default window keeps it when the client asks for a page
// without a size.
func parseWindow(r *http.Request, values url.Values) (resource.Window, error) {
	var window resource.Window
	if !values.Has("page") && !values.Has("size") {
		return window, nil
	}
	page, err := transporthttp.ParsePage(r)
	if err != nil {
		return resource.Window{}, err
	}
	if values.Has("page") {
		window.Number = page.Number
	}
	if values.Has("size") {
		window.Size = page.Size
	}
	return window, nil
}

// rejectParams refuses a recognised parameter the route does not read.
func rejectParams(values url.Values, action Action, allowed ...string) error {
	for _, name := range recognisedParams {
		if !values.Has(name) || containsString(allowed, name) {
			continue
		}
		return apperror.InvalidArgument("rest.parameter_not_allowed",
			fmt.Sprintf("parameter %q does not apply to %s", name, action))
	}
	return nil
}

func containsString(list []string, wanted string) bool {
	for _, item := range list {
		if item == wanted {
			return true
		}
	}
	return false
}

// decodeRecord reads a request body as a record, strictly.
//
// The body is read as one JSON value first and decoded a second time with
// UseNumber, so an identifier wider than 2^53 survives: decoding straight into
// map[string]any would round it, and the rounded value would address a
// different record.
func (b *binding) decodeRecord(r *http.Request) (resource.Record, error) {
	options := httpserver.StrictJSONDecodeOptions(b.settings.maxBodyBytes)
	var document json.RawMessage
	if err := httpserver.DecodeJSONBody(r, &document, options); err != nil {
		if errors.Is(err, httpserver.ErrJSONBodyEmpty) {
			return resource.Record{}, nil
		}
		return nil, bodyFailure(err)
	}
	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, apperror.InvalidArgument("rest.body_not_an_object",
			"a record is a JSON object of field names to values")
	}
	return b.normalizeRecord(decoded)
}

// normalizeRecord checks every name against the schema and converts every value
// to its field's domain, so the repository is handed the types the filter
// contract describes rather than whatever a decoder happened to produce.
//
// Names are visited in sorted order so a body with two mistakes always reports
// the same one first.
func (b *binding) normalizeRecord(decoded map[string]any) (resource.Record, error) {
	names := make([]string, 0, len(decoded))
	for name := range decoded {
		names = append(names, name)
	}
	sort.Strings(names)
	record := make(resource.Record, len(decoded))
	for _, name := range names {
		field, ok := b.collection.Schema.Field(name)
		if !ok {
			return nil, apperror.InvalidArgument("resource.unknown_field",
				fmt.Sprintf("field %q is not part of this resource", name))
		}
		value := decoded[name]
		if value == nil {
			record[name] = nil
			continue
		}
		normalized, err := resource.NormalizeValue(field.Type, value)
		if err != nil {
			return nil, apperror.InvalidArgument("resource.value_type_mismatch",
				fmt.Sprintf("field %q is %s and cannot hold %s", name, field.Type, describeValue(value)))
		}
		record[name] = normalized
	}
	return record, nil
}

// bodyFailure classifies a body decode failure the way the transport does, so a
// media type the route does not speak stays 415 rather than becoming a 400.
func bodyFailure(err error) error {
	var mediaType httpserver.UnsupportedMediaTypeError
	if errors.As(err, &mediaType) {
		return mediaType
	}
	return httpserver.JSONDecodeError{Err: err}
}

// describeValue names the JSON shape a value arrived as, so the error tells a
// client what it sent rather than only what it should have sent.
func describeValue(value any) string {
	switch value.(type) {
	case nil:
		return "nothing"
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case json.Number:
		return "a number"
	case []any:
		return "a list"
	case map[string]any:
		return "an object"
	default:
		return fmt.Sprintf("%T", value)
	}
}
