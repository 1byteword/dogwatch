package scripts

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// OutputFormat represents the output format for script results
type OutputFormat string

const (
	FormatTable OutputFormat = "table"
	FormatJSON  OutputFormat = "json"
	FormatCSV   OutputFormat = "csv"
)

// Formatter formats script results for output
type Formatter struct {
	format OutputFormat
	writer io.Writer
}

// NewFormatter creates a new result formatter
func NewFormatter(w io.Writer, format OutputFormat) *Formatter {
	return &Formatter{
		format: format,
		writer: w,
	}
}

// Format writes the result in the specified format
func (f *Formatter) Format(result *Result) error {
	switch f.format {
	case FormatJSON:
		return f.formatJSON(result)
	case FormatCSV:
		return f.formatCSV(result)
	default:
		return f.formatTable(result)
	}
}

func (f *Formatter) formatTable(result *Result) error {
	if len(result.Rows) == 0 {
		fmt.Fprintln(f.writer, "No results found.")
		return nil
	}

	// Determine column order - use script's column definition if available
	columns := result.Columns
	if len(result.Script.Columns) > 0 {
		columns = make([]string, 0, len(result.Script.Columns))
		for _, col := range result.Script.Columns {
			columns = append(columns, col.Name)
		}
	}
	if len(columns) == 0 && len(result.Rows) > 0 {
		// Fall back to keys from first row
		for k := range result.Rows[0] {
			columns = append(columns, k)
		}
		sort.Strings(columns)
	}

	tw := tabwriter.NewWriter(f.writer, 0, 0, 2, ' ', 0)

	// Header
	headers := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = strings.ToUpper(col)
	}
	fmt.Fprintln(tw, strings.Join(headers, "\t"))

	// Separator
	seps := make([]string, len(columns))
	for i, h := range headers {
		seps[i] = strings.Repeat("-", len(h))
	}
	fmt.Fprintln(tw, strings.Join(seps, "\t"))

	// Rows
	for _, row := range result.Rows {
		values := make([]string, len(columns))
		for i, col := range columns {
			values[i] = formatValue(row[col], col)
		}
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}

	tw.Flush()

	// Footer
	fmt.Fprintf(f.writer, "\n%d rows returned in %s\n", result.RowCount, result.Duration.Round(time.Millisecond))

	return nil
}

func (f *Formatter) formatJSON(result *Result) error {
	output := struct {
		Script   string                   `json:"script"`
		Rows     []map[string]interface{} `json:"rows"`
		RowCount int                      `json:"row_count"`
		Duration string                   `json:"duration"`
	}{
		Script:   result.Script.Name,
		Rows:     result.Rows,
		RowCount: result.RowCount,
		Duration: result.Duration.String(),
	}

	enc := json.NewEncoder(f.writer)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func (f *Formatter) formatCSV(result *Result) error {
	if len(result.Rows) == 0 {
		return nil
	}

	// Determine column order
	columns := result.Columns
	if len(result.Script.Columns) > 0 {
		columns = make([]string, 0, len(result.Script.Columns))
		for _, col := range result.Script.Columns {
			columns = append(columns, col.Name)
		}
	}
	if len(columns) == 0 && len(result.Rows) > 0 {
		for k := range result.Rows[0] {
			columns = append(columns, k)
		}
		sort.Strings(columns)
	}

	cw := csv.NewWriter(f.writer)

	// Header
	if err := cw.Write(columns); err != nil {
		return err
	}

	// Rows
	for _, row := range result.Rows {
		values := make([]string, len(columns))
		for i, col := range columns {
			values[i] = formatValue(row[col], col)
		}
		if err := cw.Write(values); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

// formatValue converts a value to a display string
func formatValue(v interface{}, colName string) string {
	if v == nil {
		return ""
	}

	switch val := v.(type) {
	case time.Time:
		return val.Format("2006-01-02 15:04:05")
	case time.Duration:
		return formatDuration(val)
	case float64:
		// Check if this might be a duration in milliseconds
		if strings.Contains(colName, "latency") || strings.Contains(colName, "duration") {
			return formatDuration(time.Duration(val * float64(time.Millisecond)))
		}
		if val == float64(int64(val)) {
			return fmt.Sprintf("%.0f", val)
		}
		return fmt.Sprintf("%.2f", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case int:
		return fmt.Sprintf("%d", val)
	case string:
		// Truncate long strings for table output
		if len(val) > 80 {
			return val[:77] + "..."
		}
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.2fµs", float64(d.Microseconds()))
	}
	if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

// PrintScriptList formats a list of scripts for display
func PrintScriptList(w io.Writer, scripts []*Script, verbose bool) {
	if len(scripts) == 0 {
		fmt.Fprintln(w, "No scripts available.")
		return
	}

	if verbose {
		for _, s := range scripts {
			fmt.Fprintf(w, "%s\n", s.Name)
			fmt.Fprintf(w, "  %s\n", s.Title)
			fmt.Fprintf(w, "  %s\n", s.Description)
			if len(s.Parameters) > 0 {
				fmt.Fprintln(w, "  Parameters:")
				for _, p := range s.Parameters {
					req := ""
					if p.Required {
						req = " (required)"
					}
					fmt.Fprintf(w, "    --%s=%v\t%s%s\n", p.Name, p.Default, p.Description, req)
				}
			}
			fmt.Fprintln(w)
		}
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SCRIPT\tDESCRIPTION")
		fmt.Fprintln(tw, "------\t-----------")
		for _, s := range scripts {
			desc := s.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}
			fmt.Fprintf(tw, "%s\t%s\n", s.Name, desc)
		}
		tw.Flush()
	}
}

// PrintCategories formats categories for display
func PrintCategories(w io.Writer, categories []CategoryInfo) {
	if len(categories) == 0 {
		fmt.Fprintln(w, "No categories available.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "CATEGORY\tSCRIPTS\tDESCRIPTION")
	fmt.Fprintln(tw, "--------\t-------\t-----------")
	for _, c := range categories {
		fmt.Fprintf(tw, "%s\t%d\t%s\n", c.Name, c.Count, c.Description)
	}
	tw.Flush()
}
