package memory_test

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/resource"
	"github.com/dreamsxin/go-kit/v2/resource/memory"
)

// userSchema is the fixture every test in this package queries: a readable and
// writable name and status, a readable-and-sortable numeric score, and one
// column that may be written and read by nothing.
func userSchema(t *testing.T) resource.Schema {
	t.Helper()
	schema, err := resource.NewSchema(
		resource.Field{Name: "id", Type: resource.TypeInt, Readable: true, Filterable: true, Sortable: true},
		resource.Field{Name: "name", Type: resource.TypeString, Readable: true, Filterable: true, Sortable: true, Writable: true},
		resource.Field{Name: "status", Type: resource.TypeString, Readable: true, Filterable: true, Sortable: true, Writable: true},
		resource.Field{Name: "score", Type: resource.TypeFloat, Readable: true, Filterable: true, Sortable: true, Writable: true},
		resource.Field{Name: "secret", Type: resource.TypeString, Writable: true},
	)
	if err != nil {
		t.Fatalf("NewSchema: %v", err)
	}
	return schema
}

func newStore(t *testing.T) *memory.Store {
	t.Helper()
	store, err := memory.New(userSchema(t), "id")
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	return store
}

// seed fills a store with three records and returns them as stored.
func seed(t *testing.T, store *memory.Store) []resource.Record {
	t.Helper()
	inputs := []resource.Record{
		{"name": "ling", "status": "active", "score": 3.5, "secret": "s1"},
		{"name": "wei", "status": "active", "score": 1.5, "secret": "s2"},
		{"name": "yun", "status": "done", "score": 2.0, "secret": "s3"},
	}
	stored := make([]resource.Record, 0, len(inputs))
	for _, input := range inputs {
		record, err := store.Create(context.Background(), input, resource.WriteMask{})
		if err != nil {
			t.Fatalf("Create(%v): %v", input, err)
		}
		stored = append(stored, record)
	}
	return stored
}

func codeOf(err error) string {
	var classified *apperror.Error
	if !errors.As(err, &classified) {
		return ""
	}
	return classified.ErrorCode()
}

// TestCreateAssignsTheKeyAndReturnsWhatAReaderMaySee proves two things at once:
// an omitted integer key is assigned, and the reply is the schema's projection
// rather than the storage row, so a write-only column never leaves the service.
func TestCreateAssignsTheKeyAndReturnsWhatAReaderMaySee(t *testing.T) {
	store := newStore(t)
	first, err := store.Create(context.Background(), resource.Record{"name": "ling", "secret": "s1"}, resource.WriteMask{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if first["id"] != int64(1) {
		t.Fatalf("assigned key = %v, want 1", first["id"])
	}
	if _, leaked := first["secret"]; leaked {
		t.Fatal("the reply carried a field the schema does not make readable")
	}

	second, err := store.Create(context.Background(), resource.Record{"name": "wei"}, resource.WriteMask{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if second["id"] != int64(2) {
		t.Fatalf("second key = %v, want 2", second["id"])
	}

	// A caller that chooses its own identifier moves the counter past it.
	explicit, err := store.Create(context.Background(), resource.Record{"id": 10, "name": "yun"}, resource.WriteMask{})
	if err != nil {
		t.Fatalf("Create with an explicit key: %v", err)
	}
	if explicit["id"] != 10 {
		t.Fatalf("explicit key = %v, want 10", explicit["id"])
	}
	next, err := store.Create(context.Background(), resource.Record{"name": "ming"}, resource.WriteMask{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if next["id"] != int64(11) {
		t.Fatalf("key after an explicit 10 = %v, want 11", next["id"])
	}
}

// TestCreateRefusesADuplicateKey proves the primary key is enforced rather than
// overwritten.
func TestCreateRefusesADuplicateKey(t *testing.T) {
	store := newStore(t)
	background := context.Background()
	if _, err := store.Create(background, resource.Record{"id": 1, "name": "ling"}, resource.WriteMask{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := store.Create(background, resource.Record{"id": 1, "name": "wei"}, resource.WriteMask{})
	if code := codeOf(err); code != "resource.record_exists" {
		t.Fatalf("Create over an existing key = %v, want resource.record_exists", err)
	}
}

// TestGetReportsNotFound proves a missing record is a classified NotFound, which
// is what lets a transport answer 404 instead of an empty 200.
func TestGetReportsNotFound(t *testing.T) {
	store := newStore(t)
	_, err := store.Get(context.Background(), resource.Scope{Key: resource.KeyOf("id", 404)})
	if code := codeOf(err); code != "resource.record_not_found" {
		t.Fatalf("Get = %v, want resource.record_not_found", err)
	}
	if _, err := store.Get(context.Background(), resource.Scope{Key: resource.Key{}}); codeOf(err) != "resource.key_required" {
		t.Fatalf("Get with an empty key = %v, want resource.key_required", err)
	}
}

// TestAScopeExcludesWhatTheConditionDoesNotSelect is the storage half of a
// policy: a key is an address, so a record the condition excludes has to be
// answered as missing. Answering "forbidden" would confirm that the record
// exists, which is the part a caller outside the condition may not learn.
func TestAScopeExcludesWhatTheConditionDoesNotSelect(t *testing.T) {
	store := newStore(t)
	seed(t, store)
	ctx := context.Background()
	active := resource.Condition("status", resource.OpEq, "active")

	inside := resource.Scope{Key: resource.KeyOf("id", 1), Where: active}
	if _, err := store.Get(ctx, inside); err != nil {
		t.Fatalf("Get inside the condition: %v", err)
	}
	outside := resource.Scope{Key: resource.KeyOf("id", 3), Where: active}
	if _, err := store.Get(ctx, outside); codeOf(err) != "resource.record_not_found" {
		t.Fatalf("Get outside the condition = %v, want resource.record_not_found", err)
	}
	if _, err := store.Update(ctx, outside, resource.Record{"score": 9.0}, resource.WriteMask{}); codeOf(err) != "resource.record_not_found" {
		t.Fatalf("Update outside the condition = %v, want resource.record_not_found", err)
	}
	if err := store.Destroy(ctx, outside); codeOf(err) != "resource.record_not_found" {
		t.Fatalf("Destroy outside the condition = %v, want resource.record_not_found", err)
	}
	still, err := store.Get(ctx, resource.Scope{Key: resource.KeyOf("id", 3)})
	if err != nil {
		t.Fatalf("Get after the refusals: %v", err)
	}
	if still["score"] != 2.0 {
		t.Fatalf("score = %v, want 2: a write outside the scope must not have happened", still["score"])
	}
}

// TestListAppliesTheWholeQuery is the reference behaviour: filter, sort, window
// and projection all take effect, and Total counts the matches rather than the
// window. An adapter that drops one clause answers a different question while
// still looking successful.
func TestListAppliesTheWholeQuery(t *testing.T) {
	store := newStore(t)
	seed(t, store)

	page, err := store.List(context.Background(), resource.Query{
		Filter: resource.Condition("status", resource.OpEq, "active"),
		Sort:   []resource.Sort{resource.Desc("score")},
		Fields: []string{"id", "name"},
		Window: resource.Window{Number: 1, Size: 1},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("total = %d, want 2 (the filter selects two records)", page.Total)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %v, want one", page.Items)
	}
	if page.Items[0]["name"] != "ling" {
		t.Fatalf("first item = %v, want ling (score 3.5 before 1.5)", page.Items[0])
	}
	if !page.HasNext {
		t.Fatal("HasNext is false although the second match follows")
	}
	if len(page.Items[0]) != 2 {
		t.Fatalf("item = %v, want only the projected fields", page.Items[0])
	}
}

// TestListProjectsNothingTheSchemaHides proves the default projection cannot
// carry a write-only column, even though the store holds it.
func TestListProjectsNothingTheSchemaHides(t *testing.T) {
	store := newStore(t)
	seed(t, store)

	page, err := store.List(context.Background(), resource.Query{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, item := range page.Items {
		if _, leaked := item["secret"]; leaked {
			t.Fatalf("item %v carries a field the schema does not make readable", item)
		}
	}
}

// TestListAnswersAnUnreachablePageWithNothing proves a page number near the
// integer limit is handled: the offset saturates, the window is empty, and the
// store does not slice out of bounds.
func TestListAnswersAnUnreachablePageWithNothing(t *testing.T) {
	store := newStore(t)
	seed(t, store)
	page, err := store.List(context.Background(), resource.Query{
		Window: resource.Window{Number: math.MaxInt, Size: resource.MaxPageSize},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("items = %v, want none", page.Items)
	}
	if page.Total != 3 {
		t.Fatalf("total = %d, want 3", page.Total)
	}
}

// TestListRefusesWhatItCannotHonour proves the store declines rather than
// answers a different question: an expansion it does not implement, and a filter
// on a field the resource does not declare.
func TestListRefusesWhatItCannotHonour(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	_, err := store.List(ctx, resource.Query{Appends: []string{"comments"}})
	if code := codeOf(err); code != "resource.appends_unsupported" {
		t.Fatalf("List with appends = %v, want resource.appends_unsupported", err)
	}

	_, err = store.List(ctx, resource.Query{Filter: resource.Condition("email", resource.OpEq, "x")})
	if code := codeOf(err); code != "filter.unknown_field" {
		t.Fatalf("List with an unknown field = %v, want filter.unknown_field", err)
	}
}

// TestUpdateChangesOnlyThePatch proves a patch is partial: a field it does not
// mention keeps its value, and the primary key cannot be moved.
func TestUpdateChangesOnlyThePatch(t *testing.T) {
	store := newStore(t)
	background := context.Background()
	if _, err := store.Create(background, resource.Record{"name": "ling", "status": "active", "score": 1.0}, resource.WriteMask{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := store.Update(background, resource.Scope{Key: resource.KeyOf("id", 1)}, resource.Record{"status": "done"}, resource.WriteMask{})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated["status"] != "done" {
		t.Fatalf("status = %v, want done", updated["status"])
	}
	if updated["name"] != "ling" {
		t.Fatalf("name = %v, want it untouched", updated["name"])
	}

	if _, err := store.Update(background, resource.Scope{Key: resource.KeyOf("id", 1)}, resource.Record{"id": 2}, resource.WriteMask{}); codeOf(err) != "resource.key_immutable" {
		t.Fatalf("Update of the primary key = %v, want resource.key_immutable", err)
	}
	if _, err := store.Update(background, resource.Scope{Key: resource.KeyOf("id", 1)}, resource.Record{}, resource.WriteMask{}); codeOf(err) != "resource.patch_empty" {
		t.Fatalf("Update of nothing = %v, want resource.patch_empty", err)
	}
	if _, err := store.Update(background, resource.Scope{Key: resource.KeyOf("id", 99)}, resource.Record{"status": "done"}, resource.WriteMask{}); codeOf(err) != "resource.record_not_found" {
		t.Fatalf("Update of a missing record = %v, want resource.record_not_found", err)
	}
}

// TestWritesOutsideTheMaskAreRefused proves the mask is enforced at the storage
// boundary and names what it refused, rather than storing a record the caller
// did not ask for.
func TestWritesOutsideTheMaskAreRefused(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	mask := resource.WriteMask{Allow: []string{"name"}}

	_, err := store.Create(ctx, resource.Record{"name": "ling", "secret": "s1"}, mask)
	if code := codeOf(err); code != "resource.field_not_writable" {
		t.Fatalf("Create outside the mask = %v, want resource.field_not_writable", err)
	}

	if _, err := store.Create(ctx, resource.Record{"name": "ling"}, resource.WriteMask{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = store.Update(ctx, resource.Scope{Key: resource.KeyOf("id", 1)}, resource.Record{"secret": "s9"}, mask)
	if code := codeOf(err); code != "resource.field_not_writable" {
		t.Fatalf("Update outside the mask = %v, want resource.field_not_writable", err)
	}
}

// TestCreateRefusesAFieldTheSchemaDoesNotDeclare proves an unknown field is a
// malformed request, not a column to invent.
func TestCreateRefusesAFieldTheSchemaDoesNotDeclare(t *testing.T) {
	store := newStore(t)
	_, err := store.Create(context.Background(), resource.Record{"name": "ling", "role": "admin"}, resource.WriteMask{})
	if code := codeOf(err); code != "resource.unknown_field" {
		t.Fatalf("Create with an undeclared field = %v, want resource.unknown_field", err)
	}
}

// TestDestroyReindexesTheRemainingRecords proves the key index stays correct
// after a removal: a delete that leaves stale positions makes the next update
// write to the wrong record, which is data corruption that reads as success.
func TestDestroyReindexesTheRemainingRecords(t *testing.T) {
	store := newStore(t)
	seed(t, store)
	ctx := context.Background()

	if err := store.Destroy(ctx, resource.Scope{Key: resource.KeyOf("id", 2)}); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	remaining, err := store.Get(ctx, resource.Scope{Key: resource.KeyOf("id", 3)})
	if err != nil {
		t.Fatalf("Get after a removal: %v", err)
	}
	if remaining["name"] != "yun" {
		t.Fatalf("record 3 = %v, want yun", remaining)
	}
	if _, err := store.Get(ctx, resource.Scope{Key: resource.KeyOf("id", 2)}); codeOf(err) != "resource.record_not_found" {
		t.Fatalf("Get of the removed record = %v, want resource.record_not_found", err)
	}
	page, err := store.List(ctx, resource.Query{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("total after a removal = %d, want 2", page.Total)
	}
}

// TestAggregateComputesEveryFunction proves the figures, including the empty
// set: a count of zero is a value, and a sum of nothing is not.
func TestAggregateComputesEveryFunction(t *testing.T) {
	store := newStore(t)
	seed(t, store)
	ctx := context.Background()

	count, err := store.Aggregate(ctx, resource.AggregateQuery{Function: resource.AggregateCount})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(count.Rows) != 1 || count.Rows[0].Value != 3 {
		t.Fatalf("count = %+v, want one row holding 3", count.Rows)
	}

	sum, err := store.Aggregate(ctx, resource.AggregateQuery{Function: resource.AggregateSum, Field: "score"})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if sum.Rows[0].Value != 7.0 {
		t.Fatalf("sum = %v, want 7", sum.Rows[0].Value)
	}

	average, err := store.Aggregate(ctx, resource.AggregateQuery{Function: resource.AggregateAverage, Field: "score"})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if average.Rows[0].Value != 7.0/3 {
		t.Fatalf("avg = %v, want %v", average.Rows[0].Value, 7.0/3)
	}

	maximum, err := store.Aggregate(ctx, resource.AggregateQuery{Function: resource.AggregateMax, Field: "score"})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if maximum.Rows[0].Value != 3.5 {
		t.Fatalf("max = %v, want 3.5", maximum.Rows[0].Value)
	}

	grouped, err := store.Aggregate(ctx, resource.AggregateQuery{
		Function: resource.AggregateCount,
		GroupBy:  []string{"status"},
	})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(grouped.Rows) != 2 {
		t.Fatalf("groups = %+v, want two", grouped.Rows)
	}
	if grouped.Rows[0].Group["status"] != "active" || grouped.Rows[0].Value != 2 {
		t.Fatalf("first group = %+v, want active holding 2, groups ordered by key", grouped.Rows[0])
	}

	filtered, err := store.Aggregate(ctx, resource.AggregateQuery{
		Function: resource.AggregateCount,
		Filter:   resource.Condition("status", resource.OpEq, "done"),
	})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if filtered.Rows[0].Value != 1 {
		t.Fatalf("filtered count = %v, want 1", filtered.Rows[0].Value)
	}
}

// TestAggregateOverNoRecords proves the empty set has an answer for count and no
// answer for the rest: reporting zero for a sum would claim the total is zero.
func TestAggregateOverNoRecords(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	count, err := store.Aggregate(ctx, resource.AggregateQuery{Function: resource.AggregateCount})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if count.Rows[0].Value != 0 {
		t.Fatalf("count = %v, want 0", count.Rows[0].Value)
	}
	sum, err := store.Aggregate(ctx, resource.AggregateQuery{Function: resource.AggregateSum, Field: "score"})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if sum.Rows[0].Value != nil {
		t.Fatalf("sum of nothing = %v, want nil", sum.Rows[0].Value)
	}
}

// TestStoreHandsOutCopies proves a caller cannot reach stored state through what
// it was handed.
func TestStoreHandsOutCopies(t *testing.T) {
	store := newStore(t)
	seed(t, store)
	ctx := context.Background()

	record, err := store.Get(ctx, resource.Scope{Key: resource.KeyOf("id", 1)})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	record["name"] = "tampered"

	again, err := store.Get(ctx, resource.Scope{Key: resource.KeyOf("id", 1)})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if again["name"] != "ling" {
		t.Fatalf("stored name = %v; a caller mutated the store through Get", again["name"])
	}
}

// TestStoreIsSafeForConcurrentUse exercises the lock under the race detector: a
// reference implementation that is not safe to share teaches the wrong habit.
func TestStoreIsSafeForConcurrentUse(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	var group sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for i := 0; i < 10; i++ {
				if _, err := store.Create(ctx, resource.Record{
					"name":   "ling",
					"status": "active",
					"score":  float64(i),
				}, resource.WriteMask{}); err != nil {
					t.Errorf("Create: %v", err)
					return
				}
				if _, err := store.List(ctx, resource.Query{Window: resource.Window{Size: 5}}); err != nil {
					t.Errorf("List: %v", err)
					return
				}
			}
		}(worker)
	}
	group.Wait()
	page, err := store.List(ctx, resource.Query{Window: resource.Window{Size: 1}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Total != 80 {
		t.Fatalf("total = %d, want 80", page.Total)
	}
}

// TestCancelledContextIsReported proves a cancelled query is a classified
// failure rather than a 500 from a store that kept working.
func TestCancelledContextIsReported(t *testing.T) {
	store := newStore(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.List(cancelled, resource.Query{}); codeOf(err) != "memory.canceled" {
		t.Fatalf("List = %v, want memory.canceled", err)
	}
}
