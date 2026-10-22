package tools_test

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"
)

// The tests in this file drive the declarative surface the generator emits
// alongside the typed endpoints, on the same running process and the same table.
//
// That adjacency is the point. A resource surface proven against an in-memory
// repository says nothing about the one thing it has to get right — translating
// a filter, a sort and a window into SQL that a real driver accepts — and the
// typed surface proven alone says nothing about whether two bindings over one
// table agree. Here a row written through an endpoint is read through a
// collection, and a row removed through a collection disappears from an
// endpoint.

const resourceBase = "/resources/users"

// exerciseGeneratedResourceSurface checks the declarative surface against the
// process the typed assertions just used.
//
// It is called at the end of the CRUD gate, where the table holds two rows
// (bob and carol) that the typed surface created.
func exerciseGeneratedResourceSurface(t *testing.T, baseURL string) {
	t.Helper()
	endpoint := baseURL + resourceBase

	// 1. List. The total is counted before the window, so a repository that
	// counted the window would report a different number than it returns.
	listed := callGeneratedAPI(t, http.MethodGet, endpoint+"?sort=-id", nil)
	if listed.status != http.StatusOK {
		t.Fatalf("GET %s: status %d, body %s", resourceBase, listed.status, listed.raw)
	}
	if got := jsonInt(t, listed.body, "total"); got != 2 {
		t.Errorf("GET %s: total = %d, want 2", resourceBase, got)
	}
	items := jsonList(t, listed.body, "items")
	if len(items) != 2 {
		t.Fatalf("GET %s: %d items, want 2", resourceBase, len(items))
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("GET %s: item is %T, want an object", resourceBase, items[0])
	}
	// Descending by id means the row created last comes first. An ORDER BY that
	// was built but not applied would still return two rows here.
	if got, _ := first["username"].(string); got != "carol" {
		t.Errorf("GET %s?sort=-id: first username = %q, want carol", resourceBase, got)
	}

	// 2. A window. The second page of size one holds the older row, and the
	// first page must report that another follows.
	window := callGeneratedAPI(t, http.MethodGet, endpoint+"?sort=-id&size=1&page=2", nil)
	if window.status != http.StatusOK {
		t.Fatalf("GET %s?size=1&page=2: status %d, body %s", resourceBase, window.status, window.raw)
	}
	if rows := jsonList(t, window.body, "items"); len(rows) != 1 {
		t.Errorf("GET %s?size=1&page=2: %d items, want 1", resourceBase, len(rows))
	}
	if got, _ := window.body["has_next"].(bool); got {
		t.Errorf("GET %s?size=1&page=2: has_next = true, want false", resourceBase)
	}

	// 3. A filter. $prefix becomes a LIKE with an escaped literal, which is the
	// only operator whose translation can answer with a superset when it is
	// wrong — an unescaped "%" in the operand matches everything.
	filtered := callGeneratedAPI(t, http.MethodGet,
		endpoint+"?filter="+url.QueryEscape(`{"username":{"$prefix":"bo"}}`), nil)
	if filtered.status != http.StatusOK {
		t.Fatalf("GET %s with a prefix filter: status %d, body %s", resourceBase, filtered.status, filtered.raw)
	}
	if got := jsonInt(t, filtered.body, "total"); got != 1 {
		t.Errorf("GET %s with a prefix filter: total = %d, want 1", resourceBase, got)
	}

	// 4. A filter naming a column the table does not have is refused, not
	// ignored: an ignored condition answers a broader question than the one
	// asked, which the caller cannot tell from an authorization failure.
	unknown := callGeneratedAPI(t, http.MethodGet,
		endpoint+"?filter="+url.QueryEscape(`{"password":{"$eq":"x"}}`), nil)
	if unknown.status != http.StatusBadRequest {
		t.Errorf("GET %s with an undeclared field: status %d, want 400, body %s", resourceBase, unknown.status, unknown.raw)
	}

	// 5. Create. The key is assigned by the database, so the response carries
	// it and the Location header names the new record.
	created := callGeneratedAPI(t, http.MethodPost, endpoint, map[string]any{
		"username": "dave",
		"email":    "dave@example.com",
	})
	if created.status != http.StatusCreated {
		t.Fatalf("POST %s: status %d, want 201, body %s", resourceBase, created.status, created.raw)
	}
	daveID, ok := created.body["id"].(float64)
	if !ok {
		t.Fatalf("POST %s: body has no identifier: %s", resourceBase, created.raw)
	}
	if location := created.header.Get("Location"); location != resourceBase+"/"+strconv.Itoa(int(daveID)) {
		t.Errorf("POST %s: Location = %q, want %s/%d", resourceBase, location, resourceBase, int(daveID))
	}

	// 6. A key the database assigns is not writable. The schema says so, and
	// the refusal has to come from the schema rather than from the column
	// happening to be absent from a body.
	withKey := callGeneratedAPI(t, http.MethodPost, endpoint, map[string]any{
		"id":       99,
		"username": "eve",
		"email":    "eve@example.com",
	})
	if withKey.status != http.StatusForbidden {
		t.Errorf("POST %s with an identifier: status %d, want 403, body %s", resourceBase, withKey.status, withKey.raw)
	}

	// 7. The unique constraint, through the declarative path. The generated
	// repository classifies the driver's duplicate-key error and the store maps
	// that classification to a status — the same three layers the typed path
	// goes through, reached from a different entry point.
	duplicate := callGeneratedAPI(t, http.MethodPost, endpoint, map[string]any{
		"username": "dave-again",
		"email":    "dave@example.com",
	})
	if duplicate.status != http.StatusConflict {
		t.Errorf("POST %s with a duplicate email: status %d, want 409, body %s", resourceBase, duplicate.status, duplicate.raw)
	}

	// 8. A partial update. The patch names one column and the other keeps its
	// value, which is what makes PATCH a patch and not a replace.
	daveURL := endpoint + "/" + strconv.Itoa(int(daveID))
	patched := callGeneratedAPI(t, http.MethodPatch, daveURL, map[string]any{"username": "dave-updated"})
	if patched.status != http.StatusOK {
		t.Fatalf("PATCH %s: status %d, body %s", daveURL, patched.status, patched.raw)
	}
	if got, _ := patched.body["username"].(string); got != "dave-updated" {
		t.Errorf("PATCH %s: username = %q, want dave-updated", daveURL, got)
	}
	if got, _ := patched.body["email"].(string); got != "dave@example.com" {
		t.Errorf("PATCH %s: email = %q, want the untouched dave@example.com", daveURL, got)
	}

	// 9. An update that names nothing is refused. Answering 200 would report a
	// write that never happened.
	empty := callGeneratedAPI(t, http.MethodPatch, daveURL, map[string]any{})
	if empty.status != http.StatusBadRequest {
		t.Errorf("PATCH %s with no field: status %d, want 400, body %s", daveURL, empty.status, empty.raw)
	}

	// 10. A projection, asserted on the list route on purpose.
	//
	// The single-record route projects twice — the store narrows its SELECT and
	// the handler narrows the record again — so a body that lacks a column there
	// proves only that the handler ran. A list hands the page straight back, so
	// the columns it carries are the columns the statement asked for. Without
	// this, a store that selected every column and let a caller's projection be
	// applied at encode time would read fields a policy never needed to load.
	projected := callGeneratedAPI(t, http.MethodGet, endpoint+"?fields=username&size=1", nil)
	if projected.status != http.StatusOK {
		t.Fatalf("GET %s?fields=username: status %d, body %s", resourceBase, projected.status, projected.raw)
	}
	narrowed := jsonList(t, projected.body, "items")
	if len(narrowed) != 1 {
		t.Fatalf("GET %s?fields=username: %d items, want 1", resourceBase, len(narrowed))
	}
	if row, ok := narrowed[0].(map[string]any); ok {
		if _, present := row["email"]; present {
			t.Errorf("GET %s?fields=username: item carries email: %s", resourceBase, projected.raw)
		}
		if _, present := row["username"]; !present {
			t.Errorf("GET %s?fields=username: item lacks the field that was asked for: %s", resourceBase, projected.raw)
		}
	}

	// 11. An aggregate, and the same figure grouped. The ungrouped form is the
	// one a dashboard sends; the grouped form checks that the group key comes
	// back under its own field name rather than under the value's alias.
	counted := callGeneratedAPI(t, http.MethodGet, endpoint+":aggregate?function=count", nil)
	if counted.status != http.StatusOK {
		t.Fatalf("GET %s:aggregate: status %d, body %s", resourceBase, counted.status, counted.raw)
	}
	rows := jsonList(t, counted.body, "rows")
	if len(rows) != 1 {
		t.Fatalf("GET %s:aggregate?function=count: %d rows, want 1", resourceBase, len(rows))
	}
	if row, ok := rows[0].(map[string]any); ok {
		if value, ok := row["value"].(float64); !ok || int(value) != 3 {
			t.Errorf("GET %s:aggregate?function=count: value = %v, want 3", resourceBase, row["value"])
		}
	}

	grouped := callGeneratedAPI(t, http.MethodGet, endpoint+":aggregate?function=count&group_by=username", nil)
	if grouped.status != http.StatusOK {
		t.Fatalf("GET %s:aggregate grouped: status %d, body %s", resourceBase, grouped.status, grouped.raw)
	}
	if rows := jsonList(t, grouped.body, "rows"); len(rows) != 3 {
		t.Errorf("GET %s:aggregate grouped by username: %d groups, want 3", resourceBase, len(rows))
	}

	// 12. Destroy, and the two consequences: the record is gone from the
	// collection, and gone from the typed endpoint that shares the table. The
	// second assertion is the one that makes "one table, two surfaces" a fact
	// rather than a claim.
	destroyed := callGeneratedAPI(t, http.MethodDelete, daveURL, nil)
	if destroyed.status != http.StatusNoContent {
		t.Fatalf("DELETE %s: status %d, want 204, body %s", daveURL, destroyed.status, destroyed.raw)
	}
	if gone := callGeneratedAPI(t, http.MethodGet, daveURL, nil); gone.status != http.StatusNotFound {
		t.Errorf("GET %s after delete: status %d, want 404", daveURL, gone.status)
	}
	if shared := callGeneratedAPI(t, http.MethodGet, baseURL+"/user/"+strconv.Itoa(int(daveID)), nil); shared.status != http.StatusNotFound {
		t.Errorf("GET /user/%d after a resource delete: status %d, want 404", int(daveID), shared.status)
	}

	// The delete is scoped: the rows it did not name are still there.
	remaining := callGeneratedAPI(t, http.MethodGet, endpoint, nil)
	if got := jsonInt(t, remaining.body, "total"); got != 2 {
		t.Errorf("GET %s after delete: total = %d, want 2", resourceBase, got)
	}
}
