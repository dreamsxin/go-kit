// Package memory is a reference implementation of resource.Repository.
//
// It exists for two reasons. It proves the contract can be implemented at all —
// an interface nobody has satisfied is a wish — and it gives every future
// adapter an oracle: the same query run through Store and through a SQL adapter
// must select the same records, because Store implements the semantics the
// filter documentation describes.
//
// It is not a production store. Everything lives in one process and dies with
// it; there is no persistence, no transaction across resources, and no index
// beyond the primary key.
package memory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/resource"
)

// Store is an in-memory resource.Repository.
//
// It is safe for concurrent use. Records are copied on the way in and on the way
// out, so a caller mutating what it was handed cannot change stored state.
//
// It is also the executable statement of the repository contract: every clause
// of a query is applied, Page.Total counts the matches rather than the window,
// a reply is the schema's projection, and a missing record is NotFound. The
// behaviour marker sits here rather than on the interface because a promise
// needs something that can fail when it breaks, and this is the implementation
// the tests exercise.
//
// Stable: resource.repository-contract — a repository applies every clause of the query it is given, counts the total before the window, and reports a missing record as NotFound rather than an empty one.
// Covered by: TestListAppliesTheWholeQuery, TestGetReportsNotFound, TestWritesOutsideTheMaskAreRefused, TestCreateAssignsTheKeyAndReturnsWhatAReaderMaySee
type Store struct {
	mu      sync.RWMutex
	schema  resource.Schema
	primary []string
	records []resource.Record
	index   map[string]int
	nextID  int64
}

// New builds an empty store for a schema. primary names the fields that identify
// one record; at least one is required, because a resource whose records cannot
// be addressed cannot be updated or deleted.
func New(schema resource.Schema, primary ...string) (*Store, error) {
	if len(primary) == 0 {
		return nil, apperror.InvalidArgument("memory.key_required",
			"a store needs at least one primary key field")
	}
	declared := make(map[string]struct{}, len(primary))
	for _, name := range primary {
		if _, ok := schema.Field(name); !ok {
			return nil, apperror.InvalidArgument("memory.key_unknown",
				fmt.Sprintf("primary key field %q is not part of the schema", name))
		}
		if _, duplicate := declared[name]; duplicate {
			return nil, apperror.InvalidArgument("memory.key_duplicated",
				fmt.Sprintf("primary key field %q is named twice", name))
		}
		declared[name] = struct{}{}
	}
	return &Store{
		schema:  schema,
		primary: append([]string(nil), primary...),
		index:   make(map[string]int),
		nextID:  1,
	}, nil
}

// List returns the records a query selects.
func (s *Store) List(ctx context.Context, query resource.Query) (resource.Page, error) {
	if err := contextFailure(ctx); err != nil {
		return resource.Page{}, err
	}
	if err := query.Validate(s.schema); err != nil {
		return resource.Page{}, err
	}
	if len(query.Appends) > 0 {
		// Returning records without the expansion a caller asked for is the
		// kind of partial answer that reads as "no related records".
		return resource.Page{}, apperror.Unimplemented("resource.appends_unsupported",
			fmt.Sprintf("this repository expands no relations, and %s was requested",
				strings.Join(query.Appends, ", ")))
	}
	matcher, err := resource.NewMatcher(query.Filter, s.schema)
	if err != nil {
		return resource.Page{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	matches := make([]resource.Record, 0, len(s.records))
	for _, record := range s.records {
		matched, err := matcher.Match(record)
		if err != nil {
			return resource.Page{}, err
		}
		if matched {
			matches = append(matches, record)
		}
	}
	if err := sortRecords(matches, query.Sort, s.schema); err != nil {
		return resource.Page{}, err
	}
	window := query.Window.Normalized()
	items, err := s.projectWindow(matches, window, query.Fields)
	if err != nil {
		return resource.Page{}, err
	}
	return resource.NewPage(window, len(matches), items), nil
}

// Get returns the record the scope selects.
func (s *Store) Get(ctx context.Context, scope resource.Scope) (resource.Record, error) {
	if err := contextFailure(ctx); err != nil {
		return nil, err
	}
	if err := scope.Key.Validate(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	position, err := s.lookup(scope)
	if err != nil {
		return nil, err
	}
	return s.schema.Project(s.records[position], nil)
}

// Create stores a new record and returns it as a reader may see it.
//
// Three rules are enforced here rather than left to a caller. A field outside
// the write mask is refused, a field the schema marks unwritable is refused, and
// a field the schema does not declare is refused — none of them is dropped
// silently, because a create that answers 200 having stored something else is
// worse than one that answers 400.
//
// The primary key is the one exception to the writability rule: it is the
// record's identity, so a caller may supply it. When a single integer key is
// left out, the store assigns the next one.
func (s *Store) Create(ctx context.Context, record resource.Record, mask resource.WriteMask) (resource.Record, error) {
	if err := contextFailure(ctx); err != nil {
		return nil, err
	}
	if record == nil {
		return nil, apperror.InvalidArgument("resource.record_required",
			"a create needs a record")
	}
	if err := s.checkWrite(record, mask, true); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stored := record.Clone()
	if err := s.assignKey(stored); err != nil {
		return nil, err
	}
	encoded, err := s.encode(stored)
	if err != nil {
		return nil, err
	}
	if _, exists := s.index[encoded]; exists {
		return nil, apperror.AlreadyExists("resource.record_exists",
			fmt.Sprintf("a record with that key already exists (%s)", strings.Join(s.primary, ", ")))
	}
	s.index[encoded] = len(s.records)
	s.records = append(s.records, stored)
	return s.schema.Project(stored, nil)
}

// Update changes the fields a patch names and returns the stored record.
//
// The primary key may not be changed: identity is what a key names, so moving it
// is a delete and a create, and allowing it here would let two records swap
// identities while both are addressed by the key the caller still holds.
//
// The scope's condition selects the record to change, and it is evaluated
// against the record as it stands. A patch that then moves the record out of that
// condition is a write the schema's write mask has to refuse — a store cannot see
// a policy, and inventing a rule here would disagree with every SQL adapter whose
// WHERE clause answers the same question about the same row.
func (s *Store) Update(ctx context.Context, scope resource.Scope, patch resource.Record, mask resource.WriteMask) (resource.Record, error) {
	if err := contextFailure(ctx); err != nil {
		return nil, err
	}
	if err := scope.Key.Validate(); err != nil {
		return nil, err
	}
	if len(patch) == 0 {
		return nil, apperror.InvalidArgument("resource.patch_empty",
			"an update must name at least one field")
	}
	// Checked before the schema's writability rule: "you cannot move a record's
	// identity" is a more useful answer than "that column is not writable".
	for _, name := range s.primary {
		if _, named := patch[name]; named {
			return nil, apperror.InvalidArgument("resource.key_immutable",
				fmt.Sprintf("primary key field %q cannot be changed", name))
		}
	}
	if err := s.checkWrite(patch, mask, false); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	position, err := s.lookup(scope)
	if err != nil {
		return nil, err
	}
	updated := s.records[position].Clone()
	for name, value := range patch {
		updated[name] = value
	}
	// The key fields did not change, so the index entry is still correct; the
	// encode call is what proves it rather than assuming it.
	encoded, err := s.encode(updated)
	if err != nil {
		return nil, err
	}
	if existing, found := s.index[encoded]; found && existing != position {
		return nil, apperror.AlreadyExists("resource.record_exists",
			"the updated record would collide with another record's key")
	}
	s.records[position] = updated
	return s.schema.Project(updated, nil)
}

// Destroy removes the record the scope selects.
func (s *Store) Destroy(ctx context.Context, scope resource.Scope) error {
	if err := contextFailure(ctx); err != nil {
		return err
	}
	if err := scope.Key.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	position, err := s.lookup(scope)
	if err != nil {
		return err
	}
	removed := s.encodeExisting(position)
	s.records = append(s.records[:position], s.records[position+1:]...)
	delete(s.index, removed)
	for shifted := position; shifted < len(s.records); shifted++ {
		encoded, err := s.encode(s.records[shifted])
		if err != nil {
			return err
		}
		s.index[encoded] = shifted
	}
	return nil
}

// Aggregate computes one figure over the records a filter selects.
//
// An aggregate over no matching records returns one row: a count of zero, and
// nil for every other function, because the sum of nothing has no value and
// reporting zero would claim it does.
func (s *Store) Aggregate(ctx context.Context, query resource.AggregateQuery) (resource.AggregateResult, error) {
	if err := contextFailure(ctx); err != nil {
		return resource.AggregateResult{}, err
	}
	if err := query.Validate(s.schema); err != nil {
		return resource.AggregateResult{}, err
	}
	matcher, err := resource.NewMatcher(query.Filter, s.schema)
	if err != nil {
		return resource.AggregateResult{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	groups := make(map[string]*accumulator)
	order := make([]string, 0)
	for _, record := range s.records {
		matched, err := matcher.Match(record)
		if err != nil {
			return resource.AggregateResult{}, err
		}
		if !matched {
			continue
		}
		encoded, group, err := s.groupKey(record, query.GroupBy)
		if err != nil {
			return resource.AggregateResult{}, err
		}
		bucket, found := groups[encoded]
		if !found {
			bucket = &accumulator{group: group}
			groups[encoded] = bucket
			order = append(order, encoded)
		}
		if err := bucket.add(s.schema, query, record); err != nil {
			return resource.AggregateResult{}, err
		}
	}
	if len(groups) == 0 && len(query.GroupBy) == 0 {
		return resource.AggregateResult{Rows: []resource.AggregateRow{{
			Group: resource.Record{},
			Value: emptyAggregateValue(query.Function),
		}}}, nil
	}
	sort.Strings(order)
	rows := make([]resource.AggregateRow, 0, len(order))
	for _, encoded := range order {
		rows = append(rows, resource.AggregateRow{
			Group: groups[encoded].group,
			Value: groups[encoded].value(query),
		})
	}
	return resource.AggregateResult{Rows: rows}, nil
}

// accumulator folds one group's records into a figure.
type accumulator struct {
	group   resource.Record
	count   int
	present int
	// integer records whether the summed field is an integer type, so a sum of
	// identifiers stays exact instead of going through float64.
	integer bool
	sumInt  int64
	sumReal float64
	extreme any
}

// add folds one record into the group.
func (a *accumulator) add(schema resource.Schema, query resource.AggregateQuery, record resource.Record) error {
	a.count++
	if query.Field == "" {
		return nil
	}
	field, _ := schema.Field(query.Field)
	a.integer = field.Type == resource.TypeInt
	value, present := record[query.Field]
	if !present || value == nil {
		return nil
	}
	normalized, err := resource.NormalizeValue(field.Type, value)
	if err != nil {
		return err
	}
	a.present++
	switch query.Function {
	case resource.AggregateSum, resource.AggregateAverage:
		if integer, ok := normalized.(int64); ok {
			a.sumInt += integer
		} else {
			a.sumReal += normalized.(float64)
		}
	case resource.AggregateMin, resource.AggregateMax:
		if a.extreme == nil {
			a.extreme = normalized
			return nil
		}
		order, err := resource.CompareValues(field.Type, normalized, a.extreme)
		if err != nil {
			return err
		}
		if (query.Function == resource.AggregateMin && order < 0) ||
			(query.Function == resource.AggregateMax && order > 0) {
			a.extreme = normalized
		}
	}
	return nil
}

// value returns the group's figure.
func (a *accumulator) value(query resource.AggregateQuery) any {
	switch query.Function {
	case resource.AggregateCount:
		if query.Field == "" {
			return a.count
		}
		return a.present
	case resource.AggregateSum:
		if a.present == 0 {
			return nil
		}
		if a.integer {
			return a.sumInt
		}
		return a.sumReal
	case resource.AggregateAverage:
		if a.present == 0 {
			return nil
		}
		if a.integer {
			return float64(a.sumInt) / float64(a.present)
		}
		return a.sumReal / float64(a.present)
	default:
		return a.extreme
	}
}

// emptyAggregateValue is the figure of a set with no records.
func emptyAggregateValue(function resource.AggregateFunction) any {
	if function == resource.AggregateCount {
		return 0
	}
	return nil
}

// projectWindow applies the window and the projection to the matched records.
func (s *Store) projectWindow(records []resource.Record, window resource.Window, fields []string) ([]resource.Record, error) {
	items := make([]resource.Record, 0, window.Size)
	start := window.Offset()
	if start >= int64(len(records)) {
		return items, nil
	}
	end := start + int64(window.Size)
	if end > int64(len(records)) {
		end = int64(len(records))
	}
	for _, record := range records[start:end] {
		projected, err := s.schema.Project(record, fields)
		if err != nil {
			return nil, err
		}
		items = append(items, projected)
	}
	return items, nil
}

// checkWrite refuses a write the schema or the mask does not allow.
func (s *Store) checkWrite(record resource.Record, mask resource.WriteMask, creating bool) error {
	if err := mask.Check(record); err != nil {
		return err
	}
	for _, name := range record.FieldNames() {
		field, ok := s.schema.Field(name)
		if !ok {
			return apperror.InvalidArgument("resource.unknown_field",
				fmt.Sprintf("field %q is not part of this resource", name))
		}
		if field.Writable {
			continue
		}
		if creating && s.isPrimary(name) {
			continue
		}
		return apperror.InvalidArgument("resource.field_not_writable",
			fmt.Sprintf("field %q is not writable", name))
	}
	return nil
}

// assignKey fills in a key the caller left out, when the schema allows one.
func (s *Store) assignKey(record resource.Record) error {
	if len(s.primary) != 1 {
		for _, name := range s.primary {
			if value, present := record[name]; !present || value == nil {
				return apperror.InvalidArgument("resource.key_required",
					fmt.Sprintf("a create needs primary key field %q", name))
			}
		}
		return nil
	}
	name := s.primary[0]
	if value, present := record[name]; present && value != nil {
		field, _ := s.schema.Field(name)
		if normalized, err := resource.NormalizeValue(field.Type, value); err == nil {
			if identifier, ok := normalized.(int64); ok && identifier >= s.nextID {
				// A caller that chooses its own identifiers moves the counter
				// past them, so a later assignment cannot collide.
				s.nextID = identifier + 1
			}
		}
		return nil
	}
	field, _ := s.schema.Field(name)
	if field.Type != resource.TypeInt {
		return apperror.InvalidArgument("resource.key_required",
			fmt.Sprintf("a create needs primary key field %q", name))
	}
	record[name] = s.nextID
	s.nextID++
	return nil
}

// lookup returns the position of the record a scope selects.
//
// The key finds a row and the condition decides whether this caller may reach
// it, and the second answer is NotFound rather than a refusal: a store that said
// "forbidden" would let a client enumerate the rows it cannot see by asking
// about identifiers one at a time.
//
// The condition is evaluated against the stored record, before any projection.
// A caller's projection narrows what it is shown, never what it is allowed to
// match on, so filtering a projected record would refuse rows the condition
// accepts.
func (s *Store) lookup(scope resource.Scope) (int, error) {
	key := scope.Key
	for _, name := range s.primary {
		if _, ok := key[name]; !ok {
			return 0, apperror.InvalidArgument("resource.key_incomplete",
				fmt.Sprintf("the key is missing primary key field %q", name))
		}
	}
	encoded, err := s.encode(key)
	if err != nil {
		return 0, err
	}
	position, found := s.index[encoded]
	if !found {
		return 0, notFound()
	}
	if scope.Where.IsZero() {
		return position, nil
	}
	reached, err := resource.Match(scope.Where, s.records[position], s.schema)
	if err != nil {
		return 0, err
	}
	if !reached {
		return 0, notFound()
	}
	return position, nil
}

// notFound is one answer for "the key names nothing" and "the condition excludes
// it": two spellings would make the difference observable from outside.
func notFound() error {
	return apperror.NotFound("resource.record_not_found", "no record has that key")
}

// isPrimary reports whether a field belongs to the primary key.
func (s *Store) isPrimary(name string) bool {
	for _, candidate := range s.primary {
		if candidate == name {
			return true
		}
	}
	return false
}

// encode turns key field values into one string. Every component carries its
// type, so the integer 1 and the string "1" cannot name the same record.
func (s *Store) encode(values map[string]any) (string, error) {
	parts := make([]string, 0, len(s.primary))
	for _, name := range s.primary {
		value, present := values[name]
		if !present || value == nil {
			return "", apperror.InvalidArgument("resource.key_incomplete",
				fmt.Sprintf("the key is missing primary key field %q", name))
		}
		part, err := encodeValue(s.schema, name, value)
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "\x00"), nil
}

// encodeExisting re-encodes a stored record, which is how Destroy finds the
// index entry it is about to remove.
func (s *Store) encodeExisting(position int) string {
	encoded, err := s.encode(s.records[position])
	if err != nil {
		// The record was stored through encode, so this cannot fail; if it
		// ever does, the index rebuild below repairs the entries it can.
		return ""
	}
	return encoded
}

// groupKey encodes a record's group values and returns them as a record.
func (s *Store) groupKey(record resource.Record, names []string) (string, resource.Record, error) {
	group := make(resource.Record, len(names))
	parts := make([]string, 0, len(names))
	for _, name := range names {
		value := record[name]
		group[name] = value
		part, err := encodeValue(s.schema, name, value)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "\x00"), group, nil
}

// encodeValue encodes one typed value for use in a key or a group key.
func encodeValue(schema resource.Schema, name string, value any) (string, error) {
	field, ok := schema.Field(name)
	if !ok {
		return "", apperror.InvalidArgument("resource.unknown_field",
			fmt.Sprintf("field %q is not part of this resource", name))
	}
	if value == nil {
		return "null", nil
	}
	normalized, err := resource.NormalizeValue(field.Type, value)
	if err != nil {
		return "", err
	}
	switch typed := normalized.(type) {
	case string:
		return "s:" + typed, nil
	case int64:
		return fmt.Sprintf("i:%d", typed), nil
	case float64:
		return fmt.Sprintf("f:%v", typed), nil
	case bool:
		return fmt.Sprintf("b:%t", typed), nil
	case time.Time:
		return "t:" + typed.UTC().Format(time.RFC3339Nano), nil
	default:
		return fmt.Sprintf("j:%v", typed), nil
	}
}

// sortRecords orders the matches. A record without the sort field sorts first in
// either direction, so the order of records that cannot answer the sort is the
// same whatever the direction is.
func sortRecords(records []resource.Record, orders []resource.Sort, schema resource.Schema) error {
	if len(orders) == 0 {
		return nil
	}
	var failure error
	sort.SliceStable(records, func(left, right int) bool {
		if failure != nil {
			return false
		}
		order, err := compareRecords(records[left], records[right], orders, schema)
		if err != nil {
			failure = err
			return false
		}
		return order < 0
	})
	return failure
}

// compareRecords orders two records by the first sort field that decides them.
func compareRecords(left, right resource.Record, orders []resource.Sort, schema resource.Schema) (int, error) {
	for _, order := range orders {
		field, ok := schema.Field(order.Field)
		if !ok {
			return 0, apperror.InvalidArgument("resource.unknown_field",
				fmt.Sprintf("sort field %q is not part of this resource", order.Field))
		}
		leftValue, leftPresent := left[order.Field]
		rightValue, rightPresent := right[order.Field]
		leftMissing := !leftPresent || leftValue == nil
		rightMissing := !rightPresent || rightValue == nil
		switch {
		case leftMissing && rightMissing:
			continue
		case leftMissing:
			return -1, nil
		case rightMissing:
			return 1, nil
		}
		compared, err := resource.CompareValues(field.Type, leftValue, rightValue)
		if err != nil {
			return 0, err
		}
		if compared == 0 {
			continue
		}
		if order.Descending {
			return -compared, nil
		}
		return compared, nil
	}
	return 0, nil
}

// contextFailure maps a cancelled context onto the framework's error taxonomy,
// so a caller sees a 499-class answer rather than a 500 from a dead query.
func contextFailure(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	switch err := ctx.Err(); {
	case err == nil:
		return nil
	case errors.Is(err, context.DeadlineExceeded):
		return apperror.DeadlineExceeded("memory.deadline_exceeded",
			"the query deadline passed before the store answered")
	default:
		return apperror.Canceled("memory.canceled", "the query was cancelled")
	}
}
