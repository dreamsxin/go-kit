// Package resource defines the query vocabulary a resource route speaks: a
// filter tree, the field metadata it is validated against, and the canonical
// form a condition is stored in.
//
// The filter tree is the reason this package exists. A query condition that
// cannot be serialized cannot be narrowed by a permission policy, compared in an
// audit log, or replayed from a bug report, so the condition is a value here
// rather than a string handed to a database driver. See README.md for the
// operator table and the dialect clients send.
package resource
