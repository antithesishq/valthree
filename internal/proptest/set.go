package proptest

import (
	"sort"
	"strings"
)

// set is an immutable string set.
type set struct {
	items map[string]struct{}
}

func newSet(items ...string) *set {
	s := &set{items: make(map[string]struct{}, len(items))}
	for _, item := range items {
		s.items[item] = struct{}{}
	}
	return s
}

func (s *set) With(items ...string) *set {
	m := make(map[string]struct{}, len(s.items)+len(items))
	for item := range s.items {
		m[item] = struct{}{}
	}
	for _, item := range items {
		m[item] = struct{}{}
	}
	return &set{items: m}
}

func (s *set) Contains(item string) bool {
	_, ok := s.items[item]
	return ok
}

func (s *set) Without(items ...string) *set {
	m := make(map[string]struct{}, len(s.items))
	for item := range s.items {
		m[item] = struct{}{}
	}
	for _, item := range items {
		delete(m, item)
	}
	return &set{items: m}
}

func (s *set) Len() int {
	return len(s.items)
}

func (s *set) Items() []string {
	items := make([]string, 0, len(s.items))
	for item := range s.items {
		items = append(items, item)
	}
	sort.Strings(items)
	return items
}

func (s *set) String() string {
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
