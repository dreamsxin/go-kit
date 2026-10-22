package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/dreamsxin/go-kit/v2/resource"
)

// list answers GET {collection}.
func (b *binding) list(ctx context.Context, request *request) (any, error) {
	page, err := b.collection.Repository.List(ctx, request.params.Query())
	if err != nil {
		return nil, err
	}
	return page, nil
}

// get answers GET {collection}/{id}.
//
// The projection is applied here even though the repository applies the default
// one: a caller may ask for fewer columns, and a repository that returned the
// whole row would otherwise answer a narrower question with a wider record.
//
// The condition the policies granted goes to the repository with the key, so a
// record outside them is answered as missing. Checking it here instead — fetch,
// then compare — would leave a window between the two, and would also have to
// decide what a refusal tells the caller about a row it may not see.
func (b *binding) get(ctx context.Context, request *request) (any, error) {
	record, err := b.collection.Repository.Get(ctx, b.scope(request))
	if err != nil {
		return nil, err
	}
	return b.collection.Schema.Project(record, request.params.Fields)
}

// create answers POST {collection} with the stored record and its address.
//
// The address is read from the record before the projection: a policy that hides
// the identifier from the body does not make the new record unaddressable.
func (b *binding) create(ctx context.Context, request *request) (any, error) {
	record, err := b.collection.Repository.Create(ctx, request.record, request.params.Writable)
	if err != nil {
		return nil, err
	}
	location := b.location(record)
	projected, err := b.collection.Schema.Project(record, request.params.Fields)
	if err != nil {
		return nil, err
	}
	return created{record: projected, location: location}, nil
}

// update answers PATCH {collection}/{id} with the stored record.
func (b *binding) update(ctx context.Context, request *request) (any, error) {
	record, err := b.collection.Repository.Update(ctx, b.scope(request), request.record, request.params.Writable)
	if err != nil {
		return nil, err
	}
	return b.collection.Schema.Project(record, request.params.Fields)
}

// destroy answers DELETE {collection}/{id} with no content.
func (b *binding) destroy(ctx context.Context, request *request) (any, error) {
	if err := b.collection.Repository.Destroy(ctx, b.scope(request)); err != nil {
		return nil, err
	}
	return deleted{}, nil
}

// scope pairs the key the path named with the condition the policies granted.
//
// Both travel together because a key is an address and not a permission: a
// repository asked for a record by key alone answers the question "does this row
// exist", which is a question a caller outside the constraint has no right to.
//
// Stable: rest.scope-not-found — a record a policy's condition excludes answers resource.record_not_found, exactly as a record that does not exist does.
// Covered by: TestAConditionSelectsTheRecordNotJustTheList
func (b *binding) scope(request *request) resource.Scope {
	return resource.Scope{Key: request.key, Where: request.params.Filter}
}

// aggregate answers GET {collection}:aggregate.
func (b *binding) aggregate(ctx context.Context, request *request) (any, error) {
	return b.collection.Repository.Aggregate(ctx, request.aggregate)
}

// location names the record a create just stored, so a client does not have to
// assume how an identifier is spelled.
func (b *binding) location(record resource.Record) string {
	value, ok := record[b.collection.Primary[0]]
	if !ok || value == nil {
		return ""
	}
	return b.settings.prefix + "/" + b.collection.Name + "/" + url.PathEscape(fmt.Sprint(value))
}

// created is a create's answer: the record, with 201 and its address.
type created struct {
	record   resource.Record
	location string
}

func (c created) MarshalJSON() ([]byte, error) { return json.Marshal(c.record) }

// StatusCode reports the status a create succeeds with.
func (c created) StatusCode() int { return http.StatusCreated }

// Headers carries the address of the record that was created.
func (c created) Headers() http.Header {
	if c.location == "" {
		return nil
	}
	header := http.Header{}
	header.Set("Location", c.location)
	return header
}

// deleted is a destroy's answer: no body, 204.
type deleted struct{}

// StatusCode reports the status a delete succeeds with.
func (deleted) StatusCode() int { return http.StatusNoContent }
