package fuzzy

import (
	"fmt"
	"testing"
)

// BenchmarkMatchAll measures matching one query against 50k paths, the
// matching cost of one /api/files request on an index the size of the spec's
// performance fixture.
func BenchmarkMatchAll(b *testing.B) {
	paths := make([]string, 0, 50_000)
	for area := range 100 {
		for topic := range 100 {
			for note := range 5 {
				paths = append(paths, fmt.Sprintf("area-%03d/topic-%03d/meeting-notes-%d.md", area, topic, note))
			}
		}
	}
	for _, rawQuery := range []string{"tpc42", "area 7 notes 3", "zzzz"} {
		query := ParseQuery(rawQuery)
		b.Run(rawQuery, func(b *testing.B) {
			for b.Loop() {
				for _, path := range paths {
					query.Match(path)
				}
			}
		})
	}
}
