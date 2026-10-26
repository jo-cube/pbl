package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyDuplicateBatchFailureBoundaries(t *testing.T) {
	for _, format := range []string{"kcat", "frame"} {
		for _, filter := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/bloom=%v", format, filter), func(t *testing.T) {
				db := filepath.Join(t.TempDir(), "db")
				var input strings.Builder
				for i := 0; i < 128; i++ {
					if format == "kcat" {
						fmt.Fprintf(&input, "key\t%d\t%s\n", len("committed"), "committed")
					} else {
						input.WriteString("P 3 9\nkeycommitted")
					}
				}
				if format == "kcat" {
					input.WriteString("key\t7\tpending\nkey\t-1\n")
				} else {
					input.WriteString("D 3\nkeyP 3 9\nkeyshort")
				}
				args := []string{"apply", "users", "--format", format, "--batch-size", "128", "--batch-bytes", "4MB", "--sync"}
				if filter {
					args = append(args, "--bloom-filter", "--expected-key-count", "1")
				}
				out, stderr, code := run(t, db, input.String(), args...)
				if code != 4 || out != "" || stderr == "" {
					t.Fatalf("apply out=%q err=%q code=%d", out, stderr, code)
				}
				out, stderr, code = run(t, db, "", "get", "users", "key")
				if out != "committed\n" || stderr != "" || code != 0 {
					t.Fatalf("committed value out=%q err=%q code=%d", out, stderr, code)
				}
			})
		}
	}
}

func TestImportReplacementBatchFailureBoundary(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db")
	var input strings.Builder
	for i := 0; i < 128; i++ {
		fmt.Fprintf(&input, "key\tvalue-%d\n", i)
	}
	input.WriteString("key\tpending\ninvalid\n")
	out, stderr, code := run(t, db, input.String(), "import", "users", "--format", "kv", "--batch-size", "128")
	if code != 4 || out != "" || stderr == "" {
		t.Fatalf("import out=%q err=%q code=%d", out, stderr, code)
	}
	out, stderr, code = run(t, db, "", "get", "users", "key")
	if out != "value-127\n" || stderr != "" || code != 0 {
		t.Fatalf("committed value out=%q err=%q code=%d", out, stderr, code)
	}
}
