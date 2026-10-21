// Package resource defines the query vocabulary a resource route speaks: a
// filter tree, the field metadata it is validated against, the request those are
// merged into, and the repository contract they are handed to.
//
// The filter tree is the reason this package exists. A query condition that
// cannot be serialized cannot be narrowed by a permission policy, compared in an
// audit log, or replayed from a bug report, so the condition is a value here
// rather than a string handed to a database driver.
//
// Three things follow from that, and they are the package's shape:
//
//   - Filter, Schema and Matcher define what a condition means, down to the
//     three-valued logic an adapter has to reproduce.
//   - Params and MergeParams define how a request is assembled from a default, a
//     client and a policy, with one narrowing rule per key.
//   - Repository defines what storage has to do with the result. The interface
//     lives here; the implementation lives with whoever owns the data, so this
//     package never pulls a driver into a consumer's build. resource/memory is
//     the reference implementation.
//   - Collection ties the three together: a name, a schema, a set of primary key
//     fields and a repository. It is what a transport mounts and what an
//     endpoint addresses, and it is a value rather than a registry entry — two
//     collections over one table with different schemas are two values an
//     application can hold at once.
//
// See README.md for the operator table, the dialect clients send, and the merge
// table. resource/rest is the HTTP binding, kept separate so a consumer that
// only wants the vocabulary does not build an HTTP server.
package resource
