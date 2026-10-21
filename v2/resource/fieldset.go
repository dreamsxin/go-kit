package resource

// The three field-set operations every merge rule in this package is built from.
//
// All of them distinguish nil from an empty slice, and that distinction is the
// whole safety property: nil is a layer saying "no opinion", while a non-nil
// empty slice is a layer saying "none". Merging two disjoint whitelists produces
// the second, never the first.

// intersectFields returns the fields both layers list, in the order the earlier
// layer gave them.
func intersectFields(left, right []string) []string {
	if left == nil {
		return cloneFields(right)
	}
	if right == nil {
		return cloneFields(left)
	}
	remaining := make(map[string]struct{}, len(right))
	for _, field := range right {
		remaining[field] = struct{}{}
	}
	intersection := make([]string, 0, len(left))
	for _, field := range left {
		if _, ok := remaining[field]; !ok {
			continue
		}
		delete(remaining, field)
		intersection = append(intersection, field)
	}
	return intersection
}

// unionFields returns every field either layer lists, first occurrence first.
func unionFields(left, right []string) []string {
	if left == nil && right == nil {
		return nil
	}
	union := make([]string, 0, len(left)+len(right))
	seen := make(map[string]struct{}, len(left)+len(right))
	for _, list := range [][]string{left, right} {
		for _, field := range list {
			if _, duplicate := seen[field]; duplicate {
				continue
			}
			seen[field] = struct{}{}
			union = append(union, field)
		}
	}
	return union
}

// containsField reports whether a list names one field.
func containsField(fields []string, wanted string) bool {
	for _, field := range fields {
		if field == wanted {
			return true
		}
	}
	return false
}

// cloneFields copies a field list, keeping nil as nil.
func cloneFields(fields []string) []string {
	if fields == nil {
		return nil
	}
	return append([]string{}, fields...)
}
