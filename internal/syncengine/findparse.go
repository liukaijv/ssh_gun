package syncengine

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func ParseFindPrintf(output string) ([]FileEntry, error) {
	output = strings.TrimSuffix(output, "\n")
	if output == "" {
		return nil, nil
	}

	lines := strings.Split(output, "\n")
	entries := make([]FileEntry, 0, len(lines))
	for lineNumber, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			return nil, fmt.Errorf("parse find line %d: expected 4 fields, got %d", lineNumber+1, len(fields))
		}
		if fields[0] != "f" && fields[0] != "d" {
			return nil, fmt.Errorf("parse find line %d: unsupported type %q", lineNumber+1, fields[0])
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse find line %d size: %w", lineNumber+1, err)
		}
		modTime, err := parseUnixTimestamp(fields[3])
		if err != nil {
			return nil, fmt.Errorf("parse find line %d timestamp: %w", lineNumber+1, err)
		}
		entries = append(entries, FileEntry{
			RelPath: normalizeRel(fields[1]),
			Size:    size,
			ModTime: modTime,
			IsDir:   fields[0] == "d",
		})
	}
	return entries, nil
}

func parseUnixTimestamp(value string) (time.Time, error) {
	whole, fraction, found := strings.Cut(value, ".")
	seconds, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	if !found {
		return time.Unix(seconds, 0), nil
	}
	if len(fraction) > 9 {
		fraction = fraction[:9]
	}
	fraction += strings.Repeat("0", 9-len(fraction))
	nanos, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(seconds, nanos), nil
}
