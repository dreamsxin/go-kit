package resource

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// Collection is the declaration of one addressable set of records: the name it
// answers to, the fields it exposes, the fields that identify a record, and the
// repository that stores them.
//
// It is a value, not a registry entry. Nothing here is global, so two
// collections over the same table with different schemas are two values an
// application can hold at once — which is what a narrower surface for one client
// and a wider one for another requires.
//
// The declaration is also the whole of what a transport is allowed to assume.
// A router reads Name and Primary to build paths; it never invents a field name,
// an address, or a key.
type Collection struct {
	// Name is the path segment the collection answers to. It is not the table
	// name: an application may expose `products` over `t_product_v3` without
	// the URL saying so.
	Name string
	// Schema declares the fields.
	Schema Schema
	// Primary names the fields that identify exactly one record, in the
	// order they appear in a key.
	Primary []string
	// Repository is the storage the collection reads and writes.
	Repository Repository
}

// NewCollection validates a declaration and returns it.
//
// Every check here is one a later layer cannot repair: a name that is not a
// path segment produces a route that cannot be registered, a primary key that
// is not a field produces a lookup no adapter can answer, and a nil repository
// produces a handler that panics on its first request instead of at startup.
func NewCollection(name string, schema Schema, repository Repository, primary ...string) (Collection, error) {
	collection := Collection{
		Name:       name,
		Schema:     schema,
		Primary:    append([]string(nil), primary...),
		Repository: repository,
	}
	if err := collection.Validate(); err != nil {
		return Collection{}, err
	}
	return collection, nil
}

// Validate reports whether a declaration is usable.
//
// NewCollection calls it, and so does every transport that accepts a
// Collection by value: a struct literal skips the constructor, and a
// declaration that reached a router unchecked would produce a route nobody can
// answer.
func (c Collection) Validate() error {
	if err := validateCollectionName(c.Name); err != nil {
		return err
	}
	if c.Schema.Len() == 0 {
		return apperror.InvalidArgument("resource.schema_empty",
			fmt.Sprintf("collection %q declares no fields", c.Name))
	}
	if isNilRepository(c.Repository) {
		return apperror.InvalidArgument("resource.repository_required",
			fmt.Sprintf("collection %q has no repository", c.Name))
	}
	if len(c.Primary) == 0 {
		return apperror.InvalidArgument("resource.key_required",
			fmt.Sprintf("collection %q declares no primary key field", c.Name))
	}
	declared := make(map[string]struct{}, len(c.Primary))
	for _, field := range c.Primary {
		if _, ok := c.Schema.Field(field); !ok {
			return apperror.InvalidArgument("resource.key_unknown",
				fmt.Sprintf("collection %q names primary key field %q, which is not part of the schema", c.Name, field))
		}
		if _, duplicate := declared[field]; duplicate {
			return apperror.InvalidArgument("resource.key_duplicated",
				fmt.Sprintf("collection %q names primary key field %q twice", c.Name, field))
		}
		declared[field] = struct{}{}
	}
	return nil
}

// Key builds the key that identifies one record, normalizing the value to the
// primary key field's domain.
//
// It refuses a collection whose key has several fields: one value identifies a
// record only when one field does, and a composite key needs a spelling this
// method does not have.
func (c Collection) Key(value any) (Key, error) {
	field, err := c.singleKeyField()
	if err != nil {
		return nil, err
	}
	normalized, err := NormalizeValue(field.Type, value)
	if err != nil {
		return nil, keyMalformed(field, fmt.Sprint(value))
	}
	return KeyOf(field.Name, normalized), nil
}

// KeyFromText builds the key one path segment names.
//
// A path segment is text, and the field may be an integer or an instant, so the
// conversion happens here — once, in the declaration's owner, rather than in
// every transport that reads a URL. A value that does not parse is an
// invalid-argument error naming the field, never a zero value: an identifier
// that quietly became 0 addresses a different record.
func (c Collection) KeyFromText(text string) (Key, error) {
	field, err := c.singleKeyField()
	if err != nil {
		return nil, err
	}
	if text == "" {
		return nil, apperror.InvalidArgument("resource.key_required",
			fmt.Sprintf("the path names no value for key field %q", field.Name))
	}
	var value any
	switch field.Type {
	case TypeString:
		value = text
	case TypeInt:
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, keyMalformed(field, text)
		}
		value = parsed
	case TypeFloat:
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, keyMalformed(field, text)
		}
		value = parsed
	case TypeBool:
		parsed, err := strconv.ParseBool(text)
		if err != nil {
			return nil, keyMalformed(field, text)
		}
		value = parsed
	case TypeTime:
		parsed, err := time.Parse(time.RFC3339, text)
		if err != nil {
			return nil, keyMalformed(field, text)
		}
		value = parsed
	default:
		return nil, apperror.InvalidArgument("resource.key_not_addressable",
			fmt.Sprintf("key field %q is %s, which a path segment cannot name", field.Name, field.Type))
	}
	return KeyOf(field.Name, value), nil
}

// singleKeyField returns the one field a path segment can address.
func (c Collection) singleKeyField() (Field, error) {
	if len(c.Primary) != 1 {
		return Field{}, apperror.InvalidArgument("resource.key_not_addressable",
			fmt.Sprintf("collection %q has a composite primary key (%s), which one path segment cannot name",
				c.Name, strings.Join(c.Primary, ", ")))
	}
	field, ok := c.Schema.Field(c.Primary[0])
	if !ok {
		return Field{}, apperror.InvalidArgument("resource.key_unknown",
			fmt.Sprintf("collection %q names primary key field %q, which is not part of the schema", c.Name, c.Primary[0]))
	}
	return field, nil
}

// keyMalformed names both sides of a failed conversion, so the client learns
// what it sent rather than only that it was refused.
func keyMalformed(field Field, text string) error {
	return apperror.InvalidArgument("resource.key_malformed",
		fmt.Sprintf("key field %q is %s, and %q is not a value of that type", field.Name, field.Type, text))
}

// isNilRepository reports whether a repository is absent, including the typed
// nil a pointer hidden in the interface produces.
//
// A plain nil comparison misses that case, and the miss has a cost: the
// declaration would be accepted, and the panic would arrive on the first
// request instead of at startup, where it can still stop the process with a
// message naming the collection.
func isNilRepository(repository Repository) bool {
	if repository == nil {
		return true
	}
	value := reflect.ValueOf(repository)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// validateCollectionName accepts the names that are unambiguous as a path
// segment.
//
// The restriction is not cosmetic: the name is concatenated with the action
// suffix of an aggregate route and placed in a pattern, so a name carrying `/`,
// `{`, `}` or `:` would register a route nobody meant and could shadow another
// collection's.
func validateCollectionName(name string) error {
	if name == "" {
		return apperror.InvalidArgument("resource.collection_name_invalid",
			"a collection name cannot be empty")
	}
	if name == "." || name == ".." {
		return apperror.InvalidArgument("resource.collection_name_invalid",
			fmt.Sprintf("collection name %q is a path traversal, not a name", name))
	}
	for _, character := range name {
		allowed := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' || character == '~' || character == '.'
		if !allowed {
			return apperror.InvalidArgument("resource.collection_name_invalid",
				fmt.Sprintf("collection name %q contains %q; a name is a path segment of letters, digits, '-', '_', '~' or '.'", name, character))
		}
	}
	return nil
}
