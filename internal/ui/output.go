package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

type Output struct {
	JSON bool
	Out  io.Writer
	Err  io.Writer
}

type Response struct {
	OK      bool        `json:"ok"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func (o Output) Success(message string, data interface{}) {
	if o.JSON {
		_ = json.NewEncoder(o.Out).Encode(Response{OK: true, Message: message, Data: data})
		return
	}
	if message != "" {
		fmt.Fprintln(o.Out, message)
	}
	if data != nil {
		printData(o.Out, data)
	}
}

func (o Output) Error(err error) {
	if o.JSON {
		_ = json.NewEncoder(o.Err).Encode(Response{OK: false, Message: err.Error()})
		return
	}
	fmt.Fprintln(o.Err, "error:", err.Error())
}

func (o Output) Table(headers []string, rows [][]string) {
	if o.JSON {
		_ = json.NewEncoder(o.Out).Encode(Response{OK: true, Data: rows})
		return
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, col := range row {
			if len(col) > widths[i] {
				widths[i] = len(col)
			}
		}
	}
	if len(headers) > 0 {
		fmt.Fprintln(o.Out, formatRow(headers, widths))
		separator := make([]string, len(headers))
		for i := range headers {
			separator[i] = strings.Repeat("-", widths[i])
		}
		fmt.Fprintln(o.Out, formatRow(separator, widths))
	}
	for _, row := range rows {
		fmt.Fprintln(o.Out, formatRow(row, widths))
	}
}

func formatRow(cols []string, widths []int) string {
	parts := make([]string, len(cols))
	for i, col := range cols {
		pad := widths[i] - len(col)
		if pad < 0 {
			pad = 0
		}
		parts[i] = col + strings.Repeat(" ", pad)
	}
	return strings.Join(parts, "  ")
}

func printData(w io.Writer, data interface{}) {
	switch value := data.(type) {
	case map[string]string:
		printStringMap(w, value)
	case map[string]interface{}:
		printInterfaceMap(w, value)
	case []string:
		for _, item := range value {
			fmt.Fprintln(w, item)
		}
	default:
		fmt.Fprintln(w, data)
	}
}

func printStringMap(w io.Writer, data map[string]string) {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	headers := []string{"field", "value"}
	widths := []int{len(headers[0]), len(headers[1])}
	for _, key := range keys {
		if len(key) > widths[0] {
			widths[0] = len(key)
		}
		if len(data[key]) > widths[1] {
			widths[1] = len(data[key])
		}
	}
	fmt.Fprintln(w, formatRow(headers, widths))
	fmt.Fprintln(w, formatRow([]string{strings.Repeat("-", widths[0]), strings.Repeat("-", widths[1])}, widths))
	for _, key := range keys {
		fmt.Fprintln(w, formatRow([]string{key, data[key]}, widths))
	}
}

func printInterfaceMap(w io.Writer, data map[string]interface{}) {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	headers := []string{"field", "value"}
	widths := []int{len(headers[0]), len(headers[1])}
	formatted := make([]string, len(keys))
	for i, key := range keys {
		value := formatValue(data[key])
		formatted[i] = value
		if len(key) > widths[0] {
			widths[0] = len(key)
		}
		if len(value) > widths[1] {
			widths[1] = len(value)
		}
	}
	fmt.Fprintln(w, formatRow(headers, widths))
	fmt.Fprintln(w, formatRow([]string{strings.Repeat("-", widths[0]), strings.Repeat("-", widths[1])}, widths))
	for i, key := range keys {
		fmt.Fprintln(w, formatRow([]string{key, formatted[i]}, widths))
	}
}

func formatValue(value interface{}) string {
	switch cast := value.(type) {
	case []string:
		return strings.Join(cast, ", ")
	case []interface{}:
		parts := make([]string, 0, len(cast))
		for _, item := range cast {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(value)
	}
}
