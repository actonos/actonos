package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var floors = map[string]float64{
	"github.com/actonos/actonos/internal/memory":   80,
	"github.com/actonos/actonos/internal/auth":     80,
	"github.com/actonos/actonos/internal/server":   60,
	"github.com/actonos/actonos/internal/plugin":   70,
	"github.com/actonos/actonos/internal/channels": 70,
	"github.com/actonos/actonos/internal/security": 90,
	"github.com/actonos/actonos/internal/agent":    70,
	"github.com/actonos/actonos/internal/sandbox":  70,
	"github.com/actonos/actonos/internal/tools":    60,
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cover-gate <coverage.out>")
		os.Exit(2)
	}
	pct, err := packagePercents(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	failed := false
	for pkg, floor := range floors {
		got, ok := pct[pkg]
		if !ok {
			fmt.Fprintf(os.Stderr, "cover-gate: missing package %s\n", pkg)
			failed = true
			continue
		}
		if got+0.05 < floor {
			fmt.Fprintf(os.Stderr, "cover-gate: %s is %.1f%%, floor is %.0f%%\n", pkg, got, floor)
			failed = true
			continue
		}
		fmt.Printf("cover-gate: %s %.1f%% (floor %.0f%%)\n", pkg, got, floor)
	}
	if failed {
		os.Exit(1)
	}
}

func packagePercents(path string) (map[string]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	type acc struct{ stmts, covered int }
	byPkg := map[string]*acc{}
	sc := bufio.NewScanner(f)
	if sc.Scan() {
		// skip mode: line
	}
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		fileRange := fields[0]
		stmts, _ := strconv.Atoi(fields[1])
		count, _ := strconv.Atoi(fields[2])
		file := fileRange
		if i := strings.Index(fileRange, ":"); i >= 0 {
			file = fileRange[:i]
		}
		pkg := packageOf(file)
		if pkg == "" {
			continue
		}
		a := byPkg[pkg]
		if a == nil {
			a = &acc{}
			byPkg[pkg] = a
		}
		a.stmts += stmts
		if count > 0 {
			a.covered += stmts
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for pkg, a := range byPkg {
		if a.stmts == 0 {
			out[pkg] = 100
			continue
		}
		out[pkg] = 100 * float64(a.covered) / float64(a.stmts)
	}
	return out, nil
}

func packageOf(file string) string {
	const prefix = "github.com/actonos/actonos/internal/"
	if !strings.HasPrefix(file, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(file, prefix)
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return ""
	}
	return prefix[:len(prefix)-len("internal/")] + "internal/" + rest[:slash]
}
