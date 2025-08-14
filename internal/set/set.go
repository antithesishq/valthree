// Package set provides an immutable string set.
package set

import (
	"sort"
	"strings"
)

// Set is an immutable string set.
type Set struct {
	items map[string]struct{}
}

// New constructs a new Set.
func New(items ...string) *Set {
	s := &Set{items: make(map[string]struct{}, len(items))}
	for _, item := range items {
		s.items[item] = struct{}{}
	}
	return s
}

// With returns a new Set containing with the provided items added.
func (s *Set) With(items ...string) *Set {
	m := make(map[string]struct{}, len(s.items)+len(items))
	for item := range s.items {
		m[item] = struct{}{}
	}
	for _, item := range items {
		m[item] = struct{}{}
	}
	return &Set{items: m}
}

// Contains checks if the given item is in the set.
func (s *Set) Contains(item string) bool {
	_, ok := s.items[item]
	return ok
}

// Without returns a new Set with the provided items removed.
func (s *Set) Without(items ...string) *Set {
	m := make(map[string]struct{}, len(s.items))
	for item := range s.items {
		m[item] = struct{}{}
	}
	for _, item := range items {
		delete(m, item)
	}
	return &Set{items: m}
}

// Len returns the number of items in the set.
func (s *Set) Len() int {
	return len(s.items)
}

// Items returns the items in the set as a sorted slice. It's safe for the
// caller to mutate the returned slice.
func (s *Set) Items() []string {
	items := make([]string, 0, len(s.items))
	for item := range s.items {
		items = append(items, item)
	}
	sort.Strings(items)
	return items
}

// String implements Stringer.
func (s *Set) String() string {
	var b strings.Builder
	b.WriteRune('{')
	for i, item := range s.Items() {
		if i > 0 {
			b.WriteRune(',')
			b.WriteRune(' ')
		}
		b.WriteString(item)
	}
	b.WriteRune('}')
	return b.String()
}
