// Package yamlish renders any JSON-serializable value as dense,
// human-readable YAML-like text that fits a character budget. It
// serves the yamlish and text output modes of pkg/output.
package yamlish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"unicode"
)

// Options controls the yamlish formatter behavior.
type Options struct {
	UseColor          bool
	MaxLineLen        int      // max chars per line (default 120)
	MaxStrLen         int      // max string chars before [...] truncation (default 200)
	MaxTotalLen       int      // max output chars before reduction (default 16000)
	FirstKeys         []string // keys ordered first within each group
	LastKeys          []string
	NeverInlineArrays bool // when true, always render arrays as "- item" lists
}

// Format formats any JSON-serializable value as dense, human-readable
// YAML-ish text. It renders fully expanded with line wrapping, then
// iteratively compacts from the deepest fields upward to fit within the
// total length budget.
func Format(data any, opts Options) string {
	if opts.MaxLineLen == 0 {
		opts.MaxLineLen = 120
	}
	if opts.MaxStrLen == 0 {
		opts.MaxStrLen = 200
	}
	if opts.MaxTotalLen == 0 {
		opts.MaxTotalLen = 16000
	}

	root := buildTree(normalize(data), 0)
	maxD := treeMaxDepth(root)

	structDetail := make([]int, maxD+1)
	strDetail := make([]int, maxD+1)

	// Short-circuit: if the fully expanded render
	// already fits the budget, the phases can only
	// reproduce it — skipped, and the common case for
	// small result sets.
	if full := render(root, structDetail, strDetail, nil, opts); len(full) <= opts.MaxTotalLen {
		return full
	}
	// Phase 1: find minimum structural compaction
	// that fits.
	for {
		result := render(root, structDetail, strDetail, nil, opts)
		if len(result) <= opts.MaxTotalLen {
			break
		}
		found := false
		for d := maxD; d >= 1; d-- {
			if structDetail[d] < 3 {
				structDetail[d]++
				found = true
				break
			}
		}
		if !found {
			break
		}
	}

	// Phase 2 fills the budget with progressive array
	// items, deepest arrays first (most granular per
	// char).
	const maxItemsProbe = 500
	arrayMax := make(map[int]int)
	for d := maxD; d >= 1; d-- {
		if structDetail[d] < 1 {
			continue
		}
		// renderedLen reports the output length for an
		// item cap, restoring arrayMax so probing stays
		// side-effect free.
		renderedLen := func(items int) int {
			arrayMax[d] = items
			out := render(root, structDetail, strDetail, arrayMax, opts)
			delete(arrayMax, d)
			return len(out)
		}

		// Render length is monotonic in the item cap and
		// plateaus when all items show, so
		// fits-the-budget holds for a prefix of caps and
		// binary search finds the largest fitting cap in
		// O(log 500) passes.
		lo, hi := 1, maxItemsProbe
		best := 0
		for lo <= hi {
			mid := lo + (hi-lo)/2
			if renderedLen(mid) > opts.MaxTotalLen {
				hi = mid - 1
			} else {
				best = mid
				lo = mid + 1
			}
		}
		if best > 0 {
			arrayMax[d] = best
		}
	}
	// Phase 3: string compaction deepest-first if
	// still over budget.
	for {
		result := render(root, structDetail, strDetail, arrayMax, opts)
		if len(result) <= opts.MaxTotalLen {
			break
		}
		found := false
		for d := maxD; d >= 1; d-- {
			if strDetail[d] < 3 {
				strDetail[d]++
				found = true
				break
			}
		}
		if !found {
			break
		}
	}

	result := render(root, structDetail, strDetail, arrayMax, opts)
	if len(result) > opts.MaxTotalLen {
		totalItems := countRootItems(root)
		shownItems := countShownItems(root, structDetail, arrayMax)
		result = result[:opts.MaxTotalLen]
		if totalItems > 0 {
			result += fmt.Sprintf("\n... (truncated, %d items total, %d shown)", totalItems, shownItems)
		}
	}
	return result
}

// Tree.

type nodeKind int

const (
	nScalar nodeKind = iota
	nObject
	nArray
)

type yField struct {
	key  string
	node *yNode
}

type yNode struct {
	kind   nodeKind
	depth  int
	sval   string
	fields []yField
	// inlineSorted caches the inline field order
	// (fixed per call), computed once and reused
	// across passes instead of re-sorting each time.
	inlineSorted []yField
}

type fieldClass int

const (
	classPrimitive fieldClass = iota + 1
	classInline
	classMultilinePrimitive
	classMultilineObject
	classMultilineArray
)

func buildTree(data any, depth int) *yNode {
	switch v := data.(type) {
	case nil:
		return &yNode{kind: nScalar, depth: depth, sval: "null"}
	case bool:
		return &yNode{kind: nScalar, depth: depth, sval: fmt.Sprint(v)}
	case float64:
		return &yNode{kind: nScalar, depth: depth, sval: formatFloat(v)}
	case json.Number:
		return &yNode{kind: nScalar, depth: depth, sval: v.String()}
	case string:
		return &yNode{kind: nScalar, depth: depth, sval: v}
	case []any:
		fields := make([]yField, len(v))
		for i, item := range v {
			fields[i] = yField{node: buildTree(item, depth)}
		}
		return &yNode{kind: nArray, depth: depth, fields: fields}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		fields := make([]yField, len(keys))
		for i, k := range keys {
			fields[i] = yField{key: k, node: buildTree(v[k], depth+1)}
		}
		return &yNode{kind: nObject, depth: depth, fields: fields}
	default:
		return &yNode{kind: nScalar, depth: depth, sval: fmt.Sprintf("%v", v)}
	}
}

func treeMaxDepth(n *yNode) int {
	m := n.depth
	for _, f := range n.fields {
		if d := treeMaxDepth(f.node); d > m {
			m = d
		}
	}
	return m
}

func countRootItems(n *yNode) int {
	if n.kind == nArray {
		return len(n.fields)
	}
	return 0
}

func countShownItems(n *yNode, structDetail []int, arrayMax map[int]int) int {
	if n.kind != nArray {
		return 0
	}
	sc := detailLevel(structDetail, n.depth)
	if sc < 1 {
		return len(n.fields)
	}
	maxItems := arrayMax[n.depth]
	if maxItems <= 0 {
		return 0
	}
	if maxItems < len(n.fields) {
		return maxItems
	}
	return len(n.fields)
}

// Rendering.

type renderCtx struct {
	structDetail []int
	strDetail    []int
	arrayMax     map[int]int // depth → max items to show from summarized arrays
	opts         Options
	// forceBlock suppresses the inline shortcut for
	// one renderObject call — array rendering sets it
	// so a short item does not collapse while
	// siblings expand — and clears immediately so
	// deeper nodes decide independently.
	forceBlock bool
}

func render(root *yNode, structDetail, strDetail []int, arrayMax map[int]int, opts Options) string {
	ctx := &renderCtx{
		structDetail: structDetail,
		strDetail:    strDetail,
		arrayMax:     arrayMax,
		opts:         opts,
	}
	return renderNode(root, ctx, 0, "")
}

func renderNode(n *yNode, ctx *renderCtx, indent int, prefix string) string {
	sc := detailLevel(ctx.structDetail, n.depth)
	switch n.kind {
	case nScalar:
		return prefix + formatString(n.sval, stringTruncLen(detailLevel(ctx.strDetail, n.depth), ctx.opts))
	case nArray:
		return renderArray(n, ctx, indent, prefix, sc)
	case nObject:
		return renderObject(n, ctx, indent, prefix, sc)
	}
	return ""
}

func renderObject(n *yNode, ctx *renderCtx, indent int, prefix string, sc int) string {
	if len(n.fields) == 0 {
		return prefix + emptyJSONObject
	}
	if sc >= 2 {
		return prefix + fmt.Sprintf("{%d fields}", len(n.fields))
	}

	if allFieldsScalar(n) && allChildrenHidden(n, ctx) {
		return prefix + fmt.Sprintf("{%d fields}", len(n.fields))
	}
	forceBlock := ctx.forceBlock
	ctx.forceBlock = false
	if !forceBlock {
		inline := renderObjectInline(n, ctx, prefix, sc)
		if visibleLen(inline) <= ctx.opts.MaxLineLen && !strings.Contains(inline, "\n") {
			return inline
		}
	}

	ind := strings.Repeat("  ", indent)
	type renderedField struct {
		field  yField
		valStr string
		cls    fieldClass
		inline bool
	}
	renders := make([]renderedField, 0, len(n.fields))
	for _, f := range n.fields {
		valStr := renderNode(f.node, ctx, indent+1, "")
		if valStr == "" {
			continue
		}
		line := ind + colorizeKey(f.key, ctx.opts.UseColor) + ": " + valStr
		inl := visibleLen(line) <= ctx.opts.MaxLineLen && !strings.Contains(valStr, "\n") && (valStr == "" || valStr[0] != ' ')
		renders = append(renders, renderedField{
			field:  f,
			valStr: valStr,
			cls:    classifyField(f, inl),
			inline: inl,
		})
	}
	sort.Slice(renders, func(i, j int) bool {
		if renders[i].cls != renders[j].cls {
			return renders[i].cls < renders[j].cls
		}
		return compareKeys(renders[i].field.key, renders[j].field.key, ctx.opts.FirstKeys, ctx.opts.LastKeys) < 0
	})
	var b strings.Builder
	first := true
	for _, r := range renders {
		if !first {
			b.WriteByte('\n')
		}
		first = false
		if r.inline {
			b.WriteString(ind + colorizeKey(r.field.key, ctx.opts.UseColor) + ": " + r.valStr)
		} else {
			b.WriteString(ind)
			b.WriteString(colorizeKey(r.field.key, ctx.opts.UseColor))
			b.WriteString(":\n")
			b.WriteString(renderNode(r.field.node, ctx, indent+1, ind+"  "))
		}
	}
	return b.String()
}

func renderObjectInline(n *yNode, ctx *renderCtx, prefix string, sc int) string {
	if sc >= 2 {
		return prefix + fmt.Sprintf("{%d fields}", len(n.fields))
	}
	sorted := n.inlineSorted
	if sorted == nil {
		sorted = make([]yField, len(n.fields))
		copy(sorted, n.fields)
		sort.Slice(sorted, func(i, j int) bool {
			ci := classPrimitive
			cj := classPrimitive
			if sorted[i].node.kind != nScalar {
				ci = classInline
			}
			if sorted[j].node.kind != nScalar {
				cj = classInline
			}
			if ci != cj {
				return ci < cj
			}
			return compareKeys(sorted[i].key, sorted[j].key, ctx.opts.FirstKeys, ctx.opts.LastKeys) < 0
		})
		n.inlineSorted = sorted
	}
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteByte('{')
	first := true
	for _, f := range sorted {
		val := ""
		switch f.node.kind {
		case nScalar:
			val = formatString(f.node.sval, stringTruncLen(detailLevel(ctx.strDetail, f.node.depth), ctx.opts))
		case nArray:
			switch {
			case len(f.node.fields) == 0:
				val = emptyJSONArray
			case len(f.node.fields) <= 2:
				val = renderArrayInline(ctx, "", f.node.fields)
			case detailLevel(ctx.structDetail, f.node.depth) >= 1:
				val = fmt.Sprintf("[%d items]", len(f.node.fields))
			default:
				val = renderArrayInline(ctx, "", f.node.fields)
			}
		case nObject:
			if len(f.node.fields) == 0 {
				val = emptyJSONObject
			} else {
				val = renderObjectInline(f.node, ctx, "", detailLevel(ctx.structDetail, f.node.depth))
			}
		}
		if val == "" {
			continue
		}
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString(colorizeKey(f.key, ctx.opts.UseColor))
		b.WriteString(": ")
		b.WriteString(val)
	}
	b.WriteByte('}')
	return b.String()
}

// arrayForcesBlock reports whether any array item
// would fail to render inline; when true every
// item renders block style so the array never
// mixes forms, which breaks diff-based scanning.
func arrayForcesBlock(fields []yField, ctx *renderCtx, indent int, ind string) bool {
	for _, f := range fields {
		valStr := renderNode(f.node, ctx, indent, "")
		if strings.HasPrefix(valStr, ind) && !strings.Contains(valStr, "\n") {
			valStr = valStr[len(ind):]
		}
		line := ind + "- " + valStr
		if visibleLen(line) > ctx.opts.MaxLineLen || strings.Contains(valStr, "\n") {
			return true
		}
	}
	return false
}

func renderArray(n *yNode, ctx *renderCtx, indent int, prefix string, sc int) string {
	if len(n.fields) == 0 {
		return prefix + emptyJSONArray
	}
	if sc >= 1 {
		maxItems := ctx.arrayMax[n.depth]
		if maxItems <= 0 {
			return prefix + fmt.Sprintf("[%d items]", len(n.fields))
		}
		return renderArrayProgressive(n, ctx, indent, prefix, maxItems)
	}

	if !ctx.opts.NeverInlineArrays {
		inline := renderArrayInline(ctx, prefix, n.fields)
		if visibleLen(inline) <= ctx.opts.MaxLineLen && !strings.Contains(inline, "\n") {
			return inline
		}
	}

	ind := strings.Repeat("  ", indent)

	// Pre-scan: any item that would not fit inline
	// forces block style for the whole array, using
	// the same line computation as the render loop.
	forceBlock := arrayForcesBlock(n.fields, ctx, indent, ind)

	var b strings.Builder
	for i, f := range n.fields {
		valStr := renderNode(f.node, ctx, indent, "")
		if strings.HasPrefix(valStr, ind) && !strings.Contains(valStr, "\n") {
			valStr = valStr[len(ind):]
		}
		line := ind + "- " + valStr
		if !forceBlock && visibleLen(line) <= ctx.opts.MaxLineLen && !strings.Contains(valStr, "\n") {
			b.WriteString(line)
		} else {
			b.WriteString(ind)
			b.WriteString("- \n")
			ctx.forceBlock = forceBlock
			b.WriteString(renderNode(f.node, ctx, indent+1, ind+"  "))
		}
		if i < len(n.fields)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// renderArrayProgressive shows the first maxItems
// of an over-budget array and appends [N
// remaining] for the rest.
func renderArrayProgressive(n *yNode, ctx *renderCtx, indent int, prefix string, maxItems int) string {
	remaining := 0
	showFields := n.fields
	if maxItems < len(n.fields) {
		showFields = n.fields[:maxItems]
		remaining = len(n.fields) - maxItems
	}

	ind := strings.Repeat("  ", indent)
	var b strings.Builder

	// Try inline when nothing remains.
	if remaining == 0 && !ctx.opts.NeverInlineArrays {
		inline := renderArrayInline(ctx, prefix, showFields)
		if visibleLen(inline) <= ctx.opts.MaxLineLen && !strings.Contains(inline, "\n") {
			return inline
		}
	}

	// Pre-scan: any non-fitting item forces block
	// style for the whole array, same line
	// computation as the render loop.
	forceBlock := arrayForcesBlock(showFields, ctx, indent, ind)

	for i, f := range showFields {
		valStr := renderNode(f.node, ctx, indent, "")
		if strings.HasPrefix(valStr, ind) && !strings.Contains(valStr, "\n") {
			valStr = valStr[len(ind):]
		}
		line := ind + "- " + valStr
		if !forceBlock && visibleLen(line) <= ctx.opts.MaxLineLen && !strings.Contains(valStr, "\n") {
			b.WriteString(line)
		} else {
			b.WriteString(ind)
			b.WriteString("- \n")
			ctx.forceBlock = forceBlock
			b.WriteString(renderNode(f.node, ctx, indent+1, ind+"  "))
		}
		if i < len(showFields)-1 || remaining > 0 {
			b.WriteByte('\n')
		}
	}

	if remaining > 0 {
		b.WriteString(ind)
		fmt.Fprintf(&b, "[%d remaining]", remaining)
	}
	return b.String()
}

func renderArrayInline(ctx *renderCtx, prefix string, fields []yField) string {
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteByte('[')
	for i, f := range fields {
		if i > 0 {
			b.WriteString(", ")
		}
		sc := detailLevel(ctx.structDetail, f.node.depth)
		switch f.node.kind {
		case nScalar:
			b.WriteString(formatString(f.node.sval, stringTruncLen(detailLevel(ctx.strDetail, f.node.depth), ctx.opts)))
		case nArray:
			if len(f.node.fields) == 0 {
				b.WriteString(emptyJSONArray)
			} else {
				fmt.Fprintf(&b, "[%d items]", len(f.node.fields))
			}
		case nObject:
			if len(f.node.fields) == 0 {
				b.WriteString(emptyJSONObject)
			} else {
				b.WriteString(renderObjectInline(f.node, ctx, "", sc))
			}
		}
	}
	b.WriteByte(']')
	return b.String()
}

// Compaction.

func detailLevel(detail []int, depth int) int {
	if depth >= len(detail) {
		return 0
	}
	return detail[depth]
}

func stringTruncLen(c int, opts Options) int {
	switch c {
	case 0:
		return opts.MaxStrLen
	case 1:
		return min(opts.MaxStrLen, 120)
	case 2:
		return 60
	case 3:
		return 20
	default:
		return 0
	}
}

// String formatting.

func formatString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) > maxLen {
		const marker = "[...]"
		// Cuts snap outward to space boundaries so no space-delimited
		// token (a resolved socket path) is ever split — a mid-path cut
		// once read as a different absolute path. The tail-biased split
		// protects the leaf cause error chains carry at the end.
		keepStart := maxLen * 40 / 100
		keepEnd := max(maxLen-keepStart-len(marker), 5)
		headEnd := keepStart
		if i := strings.IndexByte(s[keepStart:], ' '); i >= 0 {
			headEnd += i
		} else {
			headEnd = len(s)
		}
		tailStart := max(len(s)-keepEnd, 0)
		if i := strings.LastIndexByte(s[:tailStart], ' '); i >= 0 {
			tailStart = i + 1
		} else {
			tailStart = 0
		}
		// Visible total may exceed maxLen by the straddling tokens; the
		// compaction phases re-render tighter, so overshoot
		// self-corrects.
		switch {
		case headEnd+len(marker) < tailStart:
			s = s[:headEnd] + marker + s[tailStart:]
		case tailStart > len(marker):
			// A central token spans both cuts: no elided middle exists
			// that keeps it whole — fall back to a token-complete
			// tail-anchored cut.
			s = marker + s[tailStart:]
		default:
			// No usable boundary (one giant token): hard middle cut.
			keepEnd = min(keepEnd, len(s)-1)
			s = s[:keepStart] + marker + s[len(s)-keepEnd:]
		}
	}
	if isSimpleString(s) {
		return s
	}
	escaped := strings.ReplaceAll(s, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	escaped = strings.ReplaceAll(escaped, "\t", `\t`)
	return `"` + escaped + `"`
}

func formatFloat(v float64) string {
	if v == math.Trunc(v) && !math.IsInf(v, 0) && !math.IsNaN(v) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}

func isSimpleString(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// isGeneric reports values composed solely of the
// generic types buildTree consumes. float64 and
// other numerics are excluded so they marshal to
// json.Number and round-trip byte-identically.
func isGeneric(v any) bool {
	switch t := v.(type) {
	case nil, bool, string, json.Number:
		return true
	case map[string]any:
		for _, e := range t {
			if !isGeneric(e) {
				return false
			}
		}
		return true
	case []any:
		for _, e := range t {
			if !isGeneric(e) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func normalize(data any) any {
	// Fast path: payloads already built from generic
	// shapes skip the marshal-decode round-trip,
	// which is pure overhead.
	if isGeneric(data) {
		return data
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return data
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return data
	}
	// A typed nil slice marshals as null,
	// contradicting the caller's no-items intent — a
	// zero-row query must render as [], so top-level
	// nils promote to empty arrays for slice or map
	// sources.
	if v == nil {
		if t := reflect.TypeOf(data); t != nil {
			switch t.Kind() { //nolint:exhaustive // only container kinds need normalization
			case reflect.Slice, reflect.Array:
				return []any{}
			case reflect.Map:
				return map[string]any{}
			}
		}
	}
	return v
}

func colorizeKey(k string, useColor bool) string {
	if useColor {
		return colorKey + k + colorReset
	}
	return k
}

func allFieldsScalar(n *yNode) bool {
	for _, f := range n.fields {
		if f.node.kind != nScalar {
			return false
		}
	}
	return true
}

func allChildrenHidden(n *yNode, ctx *renderCtx) bool {
	for _, f := range n.fields {
		if f.node.kind == nScalar && stringTruncLen(detailLevel(ctx.strDetail, f.node.depth), ctx.opts) > 0 {
			return false
		}
	}
	return len(n.fields) > 0
}

func classifyField(f yField, inline bool) fieldClass {
	if f.node.kind == nScalar {
		return classPrimitive
	}
	if inline {
		return classInline
	}
	switch f.node.kind {
	case nObject:
		return classMultilineObject
	case nArray:
		if allFieldsScalar(f.node) {
			return classMultilinePrimitive
		}
		return classMultilineArray
	case nScalar:
		return classMultilineObject
	}
	return classMultilineObject
}

func compareKeys(a, b string, firstKeys, lastKeys []string) int {
	ai := keySetIndex(a, firstKeys)
	bi := keySetIndex(b, firstKeys)
	if ai >= 0 && bi >= 0 {
		return ai - bi
	}
	if ai >= 0 {
		return -1
	}
	if bi >= 0 {
		return 1
	}
	ai = keySetIndex(a, lastKeys)
	bi = keySetIndex(b, lastKeys)
	if ai >= 0 && bi >= 0 {
		return ai - bi
	}
	if ai >= 0 {
		return 1
	}
	if bi >= 0 {
		return -1
	}
	return strings.Compare(a, b)
}

func keySetIndex(key string, keys []string) int {
	for i, k := range keys {
		if k == key {
			return i
		}
	}
	return -1
}

// visibleLen returns the number of visible
// characters in s, excluding ANSI escape
// sequences (ESC [ ... m).
func visibleLen(s string) int {
	n := 0
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			if i < len(s) {
				i++ // skip 'm'
			}
			continue
		}
		n++
		i++
	}
	return n
}

// colorKey and colorReset highlight map keys when color is on.
const (
	colorKey   = "\033[36m"
	colorReset = "\033[0m"
)

// emptyJSONArray and emptyJSONObject are the degenerate renderings of
// empty containers.
const (
	emptyJSONArray  = "[]"
	emptyJSONObject = "{}"
)
