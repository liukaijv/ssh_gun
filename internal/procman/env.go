package procman

import (
	"os"
	"runtime"
	"strings"
)

// mergeProcessEnv returns a child environment based on base, with overrides
// applied. On Windows, key matching is case-insensitive.
func mergeProcessEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return append([]string(nil), base...)
	}
	folded := runtime.GOOS == "windows"
	replace := make(map[string]string, len(overrides))
	for k, v := range overrides {
		key := k
		if folded {
			key = strings.ToUpper(k)
		}
		replace[key] = k + "=" + v
	}
	out := make([]string, 0, len(base)+len(overrides))
	seen := make(map[string]struct{}, len(overrides))
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if !found {
			out = append(out, entry)
			continue
		}
		lookup := key
		if folded {
			lookup = strings.ToUpper(key)
		}
		if repl, ok := replace[lookup]; ok {
			out = append(out, repl)
			seen[lookup] = struct{}{}
			continue
		}
		out = append(out, entry)
	}
	for lookup, entry := range replace {
		if _, ok := seen[lookup]; ok {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func applyProcessEnv(cmdEnv *[]string, overrides map[string]string) {
	if len(overrides) == 0 {
		return
	}
	*cmdEnv = mergeProcessEnv(os.Environ(), overrides)
}
