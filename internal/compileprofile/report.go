package compileprofile

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// entry combines repeated compiler invocations so reports stay package-oriented.
type entry struct {
	packageName string
	durationMS  int64
	invocations int
	importChain []string
}

// Report owns the collected timing data while keeping its log and graph representation private.
type Report struct {
	baselineTotalMS int64
	profiledTotalMS int64
	entries         []entry
}

// Record writes one compiler timing record in the line format consumed by Load.
func Record(path, packageName string, duration time.Duration) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\t%d\n", packageName, duration.Milliseconds())
	return err
}

// Load aggregates the append-only compiler log into deterministic package rankings.
func Load(path string) (Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return Report{}, err
	}
	defer f.Close()

	totals := map[string]entry{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			continue
		}
		var ms int64
		if _, err := fmt.Sscanf(parts[1], "%d", &ms); err != nil {
			continue
		}
		item := totals[parts[0]]
		item.packageName = parts[0]
		item.durationMS += ms
		item.invocations++
		totals[parts[0]] = item
	}
	if err := scanner.Err(); err != nil {
		return Report{}, err
	}

	report := Report{entries: make([]entry, 0, len(totals))}
	for _, item := range totals {
		report.entries = append(report.entries, item)
	}
	sort.Slice(report.entries, func(i, j int) bool {
		if report.entries[i].durationMS != report.entries[j].durationMS {
			return report.entries[i].durationMS > report.entries[j].durationMS
		}
		return report.entries[i].packageName < report.entries[j].packageName
	})
	return report, nil
}

// Print renders the human-readable ranking while respecting the command's result limit.
func (r Report) Print(w io.Writer, top int) error {
	if len(r.entries) == 0 {
		_, err := fmt.Fprintln(w, "No packages were compiled in this build.")
		return err
	}
	if r.baselineTotalMS > 0 {
		if _, err := fmt.Fprintf(w, "Baseline build total: %dms\n", r.baselineTotalMS); err != nil {
			return err
		}
	}
	if r.profiledTotalMS > 0 {
		if _, err := fmt.Fprintf(w, "Profiled build total: %dms\n", r.profiledTotalMS); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "Compile time (packages compiled in this build):"); err != nil {
		return err
	}
	limit := len(r.entries)
	if top > 0 && top < limit {
		limit = top
	}
	for i := 0; i < limit; i++ {
		item := r.entries[i]
		if _, err := fmt.Fprintf(w, "  %2d. %-40s %4dms", i+1, item.packageName, item.durationMS); err != nil {
			return err
		}
		if item.invocations > 1 {
			if _, err := fmt.Fprintf(w, " (%dx)", item.invocations); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if len(item.importChain) > 1 {
			printImportChain(w, item.importChain)
		}
	}
	if limit < len(r.entries) {
		_, err := fmt.Fprintf(w, "      ... %d more packages omitted\n", len(r.entries)-limit)
		return err
	}
	return nil
}

// NormalizeTimings scales instrumented package timings to the uncached baseline so instrumentation overhead is not presented as compile cost.
func (r *Report) NormalizeTimings(baselineTotalMS, profiledTotalMS int64) {
	r.baselineTotalMS = baselineTotalMS
	r.profiledTotalMS = profiledTotalMS
	if baselineTotalMS <= 0 || profiledTotalMS <= 0 || len(r.entries) == 0 {
		return
	}
	for i := range r.entries {
		r.entries[i].durationMS = r.entries[i].durationMS * baselineTotalMS / profiledTotalMS
	}
}

// printImportChain indents successive imports so the dependency reason remains scannable beneath a timing entry.
func printImportChain(w io.Writer, chain []string) {
	for i, part := range chain {
		indent := "      "
		if i > 0 {
			indent += strings.Repeat("   ", i-1)
		}
		fmt.Fprintf(w, "%s└─ %s\n", indent, part)
	}
}
