package index

import (
	"fmt"
	"testing"
)

// BenchmarkScan measures a scan of roughly 50k files in 10k directories, the
// size the spec's startup budget (< 2 s) is defined against. The first
// iteration warms the OS file cache; b.Loop reports the average.
func BenchmarkScan(b *testing.B) {
	const (
		topLevelDirs   = 100
		subdirsPerTop  = 100
		filesPerSubdir = 5
	)
	root := b.TempDir()
	for top := range topLevelDirs {
		topDir := fmt.Sprintf("area-%03d", top)
		if top%10 == 0 {
			writeTestFile(b, root, topDir+"/.gitignore", "*.tmp.md\nscratch/\n!keep.tmp.md\n")
		}
		for sub := range subdirsPerTop {
			subDir := fmt.Sprintf("%s/topic-%03d", topDir, sub)
			for file := range filesPerSubdir {
				var name string
				switch file {
				case 0, 1:
					name = fmt.Sprintf("note-%d.md", file)
				case 2:
					name = "draft.tmp.md"
				default:
					name = fmt.Sprintf("asset-%d.png", file)
				}
				writeTestFile(b, root, subDir+"/"+name, "# x\n")
			}
		}
	}

	var idx *Index
	for b.Loop() {
		idx = scanForTest(b, root)
	}
	b.ReportMetric(float64(idx.FileCount), "md-files")
	b.ReportMetric(float64(idx.DirCount), "dirs")
}
