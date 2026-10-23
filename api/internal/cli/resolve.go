package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// pick finds the id that ref names among ids: an exact match, or the one id
// ref is a prefix of.
func pick(kind, ref string, ids []string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("empty %s id", kind)
	}
	if slices.Contains(ids, ref) {
		return ref, nil
	}
	var matches []string
	for _, id := range ids {
		if strings.HasPrefix(id, ref) {
			matches = append(matches, id)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no %s with id %q", kind, ref)
	case 1:
		return matches[0], nil
	}
	return "", fmt.Errorf("ambiguous %s id %q, it matches %s", kind, ref, strings.Join(matches, ", "))
}

func isUUID(s string) bool {
	id, err := uuid.Parse(s)
	return err == nil && id.String() == s
}

// place turns --before and --after (the id the item goes right before, or
// right after, among siblings, which excludes the item itself) into the
// neighbours a move change names: beforeID is the sibling ahead of the item,
// afterID the one behind it. With neither flag the item goes to the end.
func place(kind, before, after string, siblings []string) (beforeID, afterID *string, err error) {
	if before != "" && after != "" {
		return nil, nil, fmt.Errorf("--before and --after are exclusive")
	}
	switch {
	case before != "":
		id, err := pick(kind, before, siblings)
		if err != nil {
			return nil, nil, err
		}
		if i := slices.Index(siblings, id); i > 0 {
			beforeID = &siblings[i-1]
		}
		return beforeID, &id, nil
	case after != "":
		id, err := pick(kind, after, siblings)
		if err != nil {
			return nil, nil, err
		}
		if i := slices.Index(siblings, id); i+1 < len(siblings) {
			afterID = &siblings[i+1]
		}
		return &id, afterID, nil
	case len(siblings) > 0:
		return &siblings[len(siblings)-1], nil, nil
	}
	return nil, nil, nil
}
