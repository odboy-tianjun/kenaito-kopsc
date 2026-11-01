package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

// dfRow is one row of `docker system df`: a resource type, how many exist,
// how much space they use and how much of that is reclaimable.
type dfRow struct {
	Type    string
	Total   int64
	Active  int64
	Size    int64
	Reclaim int64
}

// runDockerDF implements `kopsc docker df`: `docker system df` rendered as a
// readable, sorted ranking with a total and a reclaim hint.
func runDockerDF(ctx context.Context, req BuiltinRequest) error {
	rows, err := readDockerDF(ctx)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return errors.New("docker system df 没有返回任何数据")
	}
	renderDockerDF(rows, req.Stdout)
	return nil
}

func readDockerDF(ctx context.Context) ([]dfRow, error) {
	out, err := dockerOutput(ctx, "system", "df")
	if err != nil {
		return nil, err
	}

	var rows []dfRow
	for _, line := range splitLines(out) {
		if strings.HasPrefix(strings.TrimSpace(line), "TYPE") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		row := dfRow{Type: strings.TrimSpace(fields[0])}
		row.Total = parseCount(fieldAt(fields, 1))
		row.Active = parseCount(fieldAt(fields, 2))
		row.Size, _ = parseDockerSize(fieldAt(fields, 3))
		reclaimable := fieldAt(fields, 4)
		row.Reclaim, _ = parseDockerSize(reclaimable)
		rows = append(rows, row)
	}

	// Rank by size, largest first; unknown types sort last but are kept.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Size != rows[j].Size {
			return rows[i].Size > rows[j].Size
		}
		return rows[i].Type < rows[j].Type
	})
	return rows, nil
}

func fieldAt(fields []string, i int) string {
	if i < len(fields) {
		return strings.TrimSpace(fields[i])
	}
	return ""
}

// renderDockerDF writes the ranking. The trailing "可回收" column tells the
// user how much cleanup (kopsc docker clearsafe) would free.
func renderDockerDF(rows []dfRow, w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "类型\t项\t可清理\t大小\t可回收")

	var totalSize, totalReclaim, totalItems, totalFree int64
	for _, row := range rows {
		free := row.Total - row.Active
		if free < 0 {
			free = 0
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%s\n",
			row.Type, row.Total, free, humanSize(row.Size), humanSize(row.Reclaim))
		totalSize += row.Size
		totalReclaim += row.Reclaim
		totalItems += row.Total
		totalFree += free
	}
	fmt.Fprintf(tw, "总计\t%d\t%d\t%s\t%s\n", totalItems, totalFree, humanSize(totalSize), humanSize(totalReclaim))
	_ = tw.Flush()

	if totalReclaim > 0 {
		fmt.Fprintf(w, "\n可回收 %s；清理请用 kopsc docker clearsafe（不含全部未使用镜像与卷）\n", humanSize(totalReclaim))
	} else {
		fmt.Fprintln(w, "\n没有可回收的空间")
	}
}

// parseCount reads an integer count such as "19" or "" (unknown).
func parseCount(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// parseDockerSize reads docker's size text back into bytes. Docker prints
// values such as "0B", "1.234GB", "512MB", "1.5GB (25%)" or "1.234GB (2.1GB)"
// depending on the format and version, so only the leading number and unit are
// used. A "-" or empty value means zero.
func parseDockerSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" || s == "N/A" {
		return 0, nil
	}
	// Docker appends a percentage or a parenthesised detail; drop it. A space
	// only separates number and unit (docker prints "3 MiB" in some formats),
	// so it is removed but not used to cut.
	if i := strings.IndexByte(s, '('); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	s = strings.ReplaceAll(s, " ", "")

	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	number, unit := s[:i], strings.ToUpper(strings.TrimSpace(s[i:]))
	value, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, fmt.Errorf("无法解析大小 %q", s)
	}

	multiplier := []struct {
		unit string
		base float64
	}{
		{"B", 1},
		{"KB", 1e3},
		{"MB", 1e6},
		{"GB", 1e9},
		{"TB", 1e12},
		{"PB", 1e15},
		{"KIB", 1 << 10},
		{"MIB", 1 << 20},
		{"GIB", 1 << 30},
		{"TIB", 1 << 40},
	}
	if unit == "" {
		// A bare number is bytes, as `--format {{.Size}}` may print.
		unit = "B"
	}
	for _, m := range multiplier {
		if unit == m.unit {
			return int64(value * m.base), nil
		}
	}
	return 0, fmt.Errorf("无法识别大小单位 %q", unit)
}
