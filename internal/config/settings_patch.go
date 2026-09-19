package config

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

func nodeKeys(node *unstable.Node) []string {
	var keys []string
	it := node.Key()
	for it.Next() {
		keys = append(keys, string(it.Node().Data))
	}
	return keys
}

func valueStart(data []byte, node *unstable.Node) int {
	it := node.Key()
	end := 0
	for it.Next() {
		r := it.Node().Raw
		end = int(r.Offset + r.Length)
	}
	for end < len(data) && (data[end] == ' ' || data[end] == '\t' || data[end] == '=') {
		end++
	}
	return end
}

// Scalar ranges include TOML quoting. Container ranges only contain their
// opener (or are empty), so find their end after the AST's last child.
func valueEnd(data []byte, node *unstable.Node, start int) (int, []string) {
	if node.Kind != unstable.Array && node.Kind != unstable.InlineTable {
		return int(node.Raw.Offset + node.Raw.Length), nil
	}
	end := start + 1
	var comments []string
	for child := node.Child(); child != nil; child = child.Next() {
		if child.Kind == unstable.Comment {
			comments = append(comments, strings.TrimSuffix(string(child.Data), "\r"))
			end = max(end, int(child.Raw.Offset+child.Raw.Length))
			continue
		}
		value := child
		childStart := int(child.Raw.Offset)
		if child.Kind == unstable.KeyValue {
			value = child.Value()
			childStart = valueStart(data, child)
		}
		childEnd, nested := valueEnd(data, value, childStart)
		end = max(end, childEnd)
		comments = append(comments, nested...)
	}
	close := byte(']')
	if node.Kind == unstable.InlineTable {
		close = '}'
	}
	for end < len(data) && data[end] != close {
		end++
	}
	return min(end+1, len(data)), comments
}

func patchValue(data []byte, node *unstable.Node, value string) []byte {
	start := valueStart(data, node)
	raw := node.Value().Raw
	end := int(raw.Offset + raw.Length)
	if node.Value().Kind == unstable.Array || node.Value().Kind == unstable.InlineTable {
		var comments []string
		end, comments = valueEnd(data, node.Value(), start)
		if node.Value().Kind == unstable.Array && len(comments) > 0 {
			// Preserve array comments as standalone comments inside the edited array.
			// Review explicitly discloses that element formatting/attachment may change.
			value = "[\n" + strings.Join(comments, "\n") + "\n" + strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")) + "\n]"
		}
	}
	return spliceSetting(data, start, end, settingNewlines(value, data))
}

func settingNewlines(s string, data []byte) string {
	if bytes.Contains(data, []byte("\r\n")) {
		return strings.ReplaceAll(s, "\n", "\r\n")
	}
	return s
}

func patchSetting(data []byte, section, key, value string) ([]byte, error) {
	var p unstable.Parser
	p.KeepComments = true
	p.Reset(data)
	table := ""
	insert := -1
	rootEnd := len(data)
	dotted := false
	for p.NextExpression() {
		n := p.Expression()
		if n.Kind == unstable.Comment {
			continue
		}
		keys := nodeKeys(n)
		switch n.Kind {
		case unstable.Table, unstable.ArrayTable:
			first := n.Child().Raw.Offset
			// Find the beginning of this table's line, not its quoted key.
			line := bytes.LastIndexByte(data[:first], '\n') + 1
			rootEnd = min(rootEnd, line)
			table = strings.Join(keys, ".")
			if n.Kind == unstable.ArrayTable {
				table = "[]" + table
			}
			if len(keys) == 1 && table == section {
				end := bytes.IndexByte(data[first:], '\n')
				insert = len(data)
				if end >= 0 {
					insert = int(first) + end + 1
				}
			}
		case unstable.KeyValue:
			if table == section && len(keys) == 1 && keys[0] == key {
				return patchValue(data, n, value), nil
			}
			if table != "" {
				continue
			}
			if len(keys) == 2 && keys[0] == section {
				dotted = true
				if keys[1] == key {
					return patchValue(data, n, value), nil
				}
			}
			if len(keys) != 1 || keys[0] != section || n.Value().Kind != unstable.InlineTable {
				continue
			}
			return patchInlineSetting(data, n, key, value), nil
		}
	}
	if err := p.Error(); err != nil {
		return nil, err
	}
	line := key + " = " + value + "\n"
	if insert >= 0 {
		if insert > 0 && data[insert-1] != '\n' {
			line = "\n" + line
		}
		return spliceSetting(data, insert, insert, settingNewlines(line, data)), nil
	}
	if dotted {
		line = section + "." + line
		if rootEnd > 0 && data[rootEnd-1] != '\n' {
			line = "\n" + line
		}
		return spliceSetting(data, rootEnd, rootEnd, settingNewlines(line, data)), nil
	}
	return append(data, []byte(settingNewlines(fmt.Sprintf("\n[%s]\n%s", section, line), data))...), nil
}

func spliceSetting(data []byte, start, end int, value string) []byte {
	out := make([]byte, 0, len(data)+len(value))
	out = append(out, data[:start]...)
	out = append(out, value...)
	return append(out, data[end:]...)
}

func patchInlineSetting(data []byte, n *unstable.Node, key, value string) []byte {
	var last *unstable.Node
	for child := n.Value().Child(); child != nil; child = child.Next() {
		if child.Kind != unstable.KeyValue {
			continue
		}
		childKeys := nodeKeys(child)
		if len(childKeys) == 1 && childKeys[0] == key {
			return patchValue(data, child, value)
		}
		last = child
	}
	insert := valueStart(data, n) + 1
	prefix := ""
	if last != nil {
		insert, _ = valueEnd(data, last.Value(), valueStart(data, last))
		prefix = ","
	}
	// Insert before any existing trailing comma or comment, using the AST's
	// last value boundary rather than guessing from the table's raw suffix.
	return spliceSetting(data, insert, insert, settingNewlines(prefix+" "+key+" = "+value+" ", data))
}
