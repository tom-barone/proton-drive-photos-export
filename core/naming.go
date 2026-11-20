package core

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// photoPath returns a deterministic destination path. The timestamp is
// in UTC so the name does not depend on the computer's time zone. If a
// different link already claimed the same path this run, the link ID
// disambiguates.
func photoPath(dir, name string, ts time.Time, linkID string, claimed map[string]string) string {
	name = sanitizeName(name)
	stamp := ts.UTC().Format("20060102_150405")
	path := filepath.Join(dir, stamp+"_"+name)
	if owner, ok := claimed[path]; ok && owner != linkID {
		path = filepath.Join(dir, stamp+"_"+linkID[:min(8, len(linkID))]+"_"+name)
	}
	claimed[path] = linkID
	return path
}

var reservedName = regexp.MustCompile(`^(?i)(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(\.|$)`)

// sanitizeName makes a decrypted file name safe on every target OS,
// replacing characters Windows forbids and dodging its reserved names.
func sanitizeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return "unnamed"
	}
	if reservedName.MatchString(name) {
		return "_" + name
	}
	return name
}
