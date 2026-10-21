// Command odol-corpus parses every .p3d under a directory and reports how the
// parsers cope: the format mix, failures, panics, partially parsed LODs and
// timing. Run it before and after a parser change, and compare the two runs,
// to catch regressions across a large set of real models.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jmhobbs/odol/p3d/detector"
	"github.com/jmhobbs/odol/p3d/mlod"
	"github.com/jmhobbs/odol/p3d/model"
	"github.com/jmhobbs/odol/p3d/odol"
)

// partialParseProperty is the LOD property p3d/odol sets when a LOD only
// parsed partially.
const partialParseProperty = "odol_partial_parse"

// maxListedFailures caps how many failing or changed files the text summary
// names.
const maxListedFailures = 50

type result struct {
	Path        string        `json:"path"`
	Format      string        `json:"format"`
	Error       string        `json:"error,omitempty"`
	Panic       bool          `json:"panic,omitempty"`
	LODs        int           `json:"lods"`
	PartialLODs int           `json:"partialLods"`
	Fingerprint string        `json:"fingerprint,omitempty"`
	Duration    time.Duration `json:"durationNs"`
}

type options struct {
	jobs        int
	jsonPath    string
	comparePath string
}

func main() {
	var opts options
	// The ODOL parser can briefly allocate up to 128 MiB while probing for a
	// structure boundary, so a high job count can exhaust memory; that is a
	// fatal runtime error, not a recoverable panic.
	flag.IntVar(&opts.jobs, "jobs", 4, "number of files to parse in parallel (each can briefly need 128 MiB)")
	flag.StringVar(&opts.jsonPath, "json", "", "also write every per-file result to this JSON file")
	flag.StringVar(&opts.comparePath, "compare", "", "JSON results from an earlier run to report changes against")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [--jobs N] [--json results.json] [--compare earlier.json] <dir>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 || opts.jobs < 1 {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(flag.Arg(0), opts, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root string, opts options, stdout io.Writer) error {
	var earlier []result
	if opts.comparePath != "" {
		data, err := os.ReadFile(opts.comparePath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &earlier); err != nil {
			return fmt.Errorf("read %s: %w", opts.comparePath, err)
		}
	}

	paths, err := findP3Ds(root)
	if err != nil {
		return err
	}

	start := time.Now()
	results := parseAll(paths, opts.jobs)
	elapsed := time.Since(start)

	if err := writeSummary(stdout, root, results, elapsed); err != nil {
		return err
	}
	if opts.comparePath != "" {
		if err := writeComparison(stdout, opts.comparePath, earlier, results); err != nil {
			return err
		}
	}

	if opts.jsonPath == "" {
		return nil
	}
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(opts.jsonPath, data, 0o644)
}

// findP3Ds returns every .p3d file under root, sorted so runs are comparable.
func findP3Ds(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".p3d") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// parseAll parses paths on jobs goroutines. Results keep the order of paths.
func parseAll(paths []string, jobs int) []result {
	results := make([]result, len(paths))
	next := make(chan int)

	var wg sync.WaitGroup
	for range jobs {
		wg.Go(func() {
			for i := range next {
				results[i] = parseOne(paths[i])
			}
		})
	}
	for i := range paths {
		next <- i
	}
	close(next)
	wg.Wait()

	return results
}

// parseOne parses a single file, turning a parser panic into a failed result
// so one bad file can't stop the run.
func parseOne(path string) (res result) {
	res.Path = path
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			res.Panic = true
			res.Error = fmt.Sprint(r)
		}
		res.Duration = time.Since(start)
	}()

	format, err := detector.DetectFile(path)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Format = format.String()

	var parsed *model.Model
	switch format.Family {
	case detector.P3DFamilyMLOD:
		parsed, err = mlod.ParseFile(path)
	case detector.P3DFamilyODOL:
		parsed, err = odol.ParseFile(path)
	default:
		err = fmt.Errorf("unsupported format: %s", format.String())
	}
	if err != nil {
		res.Error = err.Error()
		return res
	}

	res.LODs = len(parsed.LODs)
	for _, lod := range parsed.LODs {
		if _, ok := lod.Properties[partialParseProperty]; ok {
			res.PartialLODs++
		}
	}
	res.Fingerprint = fingerprint(parsed)
	return res
}

// fingerprint summarises the geometry the parser produced, so runs can be
// compared file by file: a parser change that alters any LOD's resolution or
// vertex, face, selection, proxy or UV set count changes it.
func fingerprint(m *model.Model) string {
	h := fnv.New64a()
	for _, lod := range m.LODs {
		_, _ = fmt.Fprintf(h, "%g:%d:%d:%d:%d:%d;", lod.Resolution, len(lod.Vertices), len(lod.Faces),
			len(lod.Selections), len(lod.Proxies), len(lod.UVSets))
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// writeComparison reports files whose outcome differs from an earlier run:
// newly failing or fixed, a changed fingerprint, or a changed partial LOD
// count. Files present in only one run are counted but not listed.
func writeComparison(w io.Writer, earlierPath string, earlier, current []result) error {
	before := make(map[string]result, len(earlier))
	for _, res := range earlier {
		before[res.Path] = res
	}

	var changes []string
	matched := 0
	for _, now := range current {
		was, ok := before[now.Path]
		if !ok {
			continue
		}
		matched++
		switch {
		case was.Error == "" && now.Error != "":
			changes = append(changes, fmt.Sprintf("now fails: %s: %s", now.Path, now.Error))
		case was.Error != "" && now.Error == "":
			changes = append(changes, fmt.Sprintf("now parses: %s", now.Path))
		case was.Fingerprint != now.Fingerprint:
			changes = append(changes, fmt.Sprintf("geometry changed: %s", now.Path))
		case was.PartialLODs != now.PartialLODs:
			changes = append(changes, fmt.Sprintf("partial LODs %d -> %d: %s", was.PartialLODs, now.PartialLODs, now.Path))
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\nCompared with %s: %d files in both runs, %d only in the earlier run, %d only in this run, %d changed\n",
		earlierPath, matched, len(earlier)-matched, len(current)-matched, len(changes))
	for i, change := range changes {
		if i == maxListedFailures {
			fmt.Fprintf(&b, "  ... and %d more\n", len(changes)-i)
			break
		}
		fmt.Fprintf(&b, "  %s\n", change)
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func writeSummary(w io.Writer, root string, results []result, elapsed time.Duration) error {
	formats := map[string]int{}
	var failed, panicked, withPartial, partialLODs int
	durations := make([]time.Duration, 0, len(results))
	for _, res := range results {
		format := res.Format
		if format == "" {
			format = "(undetected)"
		}
		formats[format]++
		if res.Error != "" {
			failed++
		}
		if res.Panic {
			panicked++
		}
		if res.PartialLODs > 0 {
			withPartial++
			partialLODs += res.PartialLODs
		}
		durations = append(durations, res.Duration)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	var b strings.Builder
	fmt.Fprintf(&b, "Corpus: %s\n", root)
	fmt.Fprintf(&b, "Files: %d (%s wall)\n", len(results), elapsed.Round(time.Millisecond))
	fmt.Fprintf(&b, "Parsed: %d, failed: %d (panics: %d)\n", len(results)-failed, failed, panicked)
	fmt.Fprintf(&b, "Models with partial LODs: %d (%d LODs)\n", withPartial, partialLODs)
	if len(durations) > 0 {
		fmt.Fprintf(&b, "Per file: p50 %s, p95 %s, max %s\n",
			percentile(durations, 50), percentile(durations, 95), durations[len(durations)-1].Round(time.Microsecond))
	}

	fmt.Fprintf(&b, "\nFormats:\n")
	names := make([]string, 0, len(formats))
	for name := range formats {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if formats[names[i]] != formats[names[j]] {
			return formats[names[i]] > formats[names[j]]
		}
		return names[i] < names[j]
	})
	for _, name := range names {
		fmt.Fprintf(&b, "  %-16s %d\n", name, formats[name])
	}

	if failed > 0 {
		fmt.Fprintf(&b, "\nFailures:\n")
		listed := 0
		for _, res := range results {
			if res.Error == "" {
				continue
			}
			if listed == maxListedFailures {
				fmt.Fprintf(&b, "  ... and %d more\n", failed-listed)
				break
			}
			kind := "error"
			if res.Panic {
				kind = "panic"
			}
			fmt.Fprintf(&b, "  %s: %s: %s\n", kind, res.Path, res.Error)
			listed++
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// percentile returns the p-th percentile of sorted durations (nearest rank).
func percentile(sorted []time.Duration, p int) time.Duration {
	rank := (p*len(sorted) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1].Round(time.Microsecond)
}
