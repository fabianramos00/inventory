//go:build contract

package contract

import (
	"fmt"
	"strings"
	"time"
)

func renderReport(legacy string, failures []failure, skips []skipNote) string {
	var b strings.Builder
	b.WriteString("# Contract parity report\n\n")
	fmt.Fprintf(&b, "- Generated: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Legacy target: %s\n", legacy)
	fmt.Fprintf(&b, "- Result: **FAIL** (%d fixture(s), %d skip(s))\n\n", len(failures), len(skips))

	if len(skips) > 0 {
		b.WriteString("## Skipped fixtures\n\n")
		for _, skip := range skips {
			fmt.Fprintf(&b, "- `%s` / `%s`: %s\n", skip.Module, skip.Fixture, skip.Reason)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Mismatches\n\n")
	for _, item := range failures {
		fmt.Fprintf(&b, "### %s / %s\n\n", item.Module, item.Fixture)
		fmt.Fprintf(&b, "`%s`\n\n", item.Request)
		for _, detail := range item.Details {
			fmt.Fprintf(&b, "- %s\n", detail)
		}
		b.WriteString("\n")
	}
	return b.String()
}
