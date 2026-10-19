// Copyright 2015-2025 Brett Vickers.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etree

import (
	"bytes"
	"errors"
	"slices"
	"strings"
)

// xmlNamespaceURI is the namespace URI permanently bound to the reserved
// "xml" prefix.
const xmlNamespaceURI = "http://www.w3.org/XML/1998/namespace"

// errRelativeNamespaceURI is returned when canonicalization encounters a
// namespace name that is not an absolute URI reference, as required by the
// Canonical XML specification.
var errRelativeNamespaceURI = errors.New("etree: canonical XML requires an absolute namespace URI")

// CanonicalXML serializes the document into canonical XML, as defined by
// W3C Canonical XML Version 1.1 (the inclusive canonicalization method).
// The output is UTF-8 encoded and contains no byte order mark, XML
// declaration, or document type declaration, making it suitable for direct
// byte comparison or hashing.
//
// If comments is true, comments are retained in the canonical output;
// otherwise they are omitted.
//
// Canonicalization is a read-only operation: the document's tree, namespace
// bindings, attribute order, child order, and WriteSettings are never
// modified. An error is returned without producing output if the tree
// contains a namespace declaration with a relative namespace URI, which is
// not permitted by the specification. No digital signature services are
// provided.
func (d *Document) CanonicalXML(comments bool) ([]byte, error) {
	c := &canonicalizer{comments: comments}

	root := d.Root()
	if root == nil {
		// Without a document element there is nothing to canonicalize.
		return []byte{}, nil
	}
	rootIndex := root.Index()

	for i, tok := range d.Child {
		switch t := tok.(type) {
		case *Element:
			if t != root {
				continue // only the single document element is serialized
			}
			initialScope := map[string]string{"xml": xmlNamespaceURI}
			if err := c.writeElement(t, initialScope, true, nil); err != nil {
				return nil, err
			}
		case *Comment:
			if !comments {
				continue
			}
			if i < rootIndex {
				c.writeComment(t)
				c.buf.WriteByte('\n')
			} else {
				c.buf.WriteByte('\n')
				c.writeComment(t)
			}
		case *ProcInst:
			// The XML declaration is reported by the XML parser as a
			// processing instruction with target "xml"; it is never part of
			// the canonical form.
			if t.Target == "xml" {
				continue
			}
			if i < rootIndex {
				c.writeProcInst(t)
				c.buf.WriteByte('\n')
			} else {
				c.buf.WriteByte('\n')
				c.writeProcInst(t)
			}
		}
	}
	return c.buf.Bytes(), nil
}

// CanonicalXML serializes the element and the complete subtree below it into
// canonical XML, as defined by W3C Canonical XML Version 1.1 (the inclusive
// canonicalization method). The element must remain connected to the tree it
// belongs to; the namespace context, inheritable xml:lang and xml:space
// attributes, and xml:base path resolution contributed by its real ancestor
// chain are taken into account even though the ancestors are not rendered.
// The xml:id attribute is not inherited.
//
// If comments is true, comments within the subtree are retained; otherwise
// they are omitted. The output is UTF-8 encoded with no byte order mark, XML
// declaration, or document type declaration.
//
// As with Document.CanonicalXML, the operation never modifies the tree. An
// error is returned without producing output if the subtree or its ancestor
// context contains a namespace declaration with a relative namespace URI.
func (e *Element) CanonicalXML(comments bool) ([]byte, error) {
	if e == nil {
		return []byte{}, nil
	}

	// Build the ancestor chain from the outermost element to the element's
	// immediate parent. The document's embedded element, if present, is
	// included but never carries attributes.
	var chain []*Element
	for p := e.parent; p != nil; p = p.parent {
		chain = append(chain, p)
	}
	slices.Reverse(chain)

	// Build the namespace scope inherited from the ancestor chain, starting
	// with the implicit binding of the reserved "xml" prefix.
	inheritedScope := map[string]string{"xml": xmlNamespaceURI}
	for _, a := range chain {
		if err := applyNamespaceDecls(a, inheritedScope); err != nil {
			return nil, err
		}
	}

	fix, err := buildApexFix(e, chain)
	if err != nil {
		return nil, err
	}

	c := &canonicalizer{comments: comments}
	if err := c.writeElement(e, inheritedScope, true, fix); err != nil {
		return nil, err
	}
	return c.buf.Bytes(), nil
}

// canonicalizer holds the per-call state of a single canonicalization. A new
// canonicalizer is created for every call, so independent calls can never
// share state.
type canonicalizer struct {
	comments bool
	buf      bytes.Buffer
}

// apexFix holds document-subset fixups applied to the apex element of a
// canonicalized subtree. A nil apexFix indicates no fixup is required.
type apexFix struct {
	// inherit maps the local name of an inheritable xml-namespace attribute
	// ("lang" or "space") to the value inherited from an omitted ancestor.
	inherit map[string]string

	// baseRender reports whether a fixed xml:base attribute must be
	// rendered on the apex.
	baseRender bool

	// baseValue is the fixed xml:base value to render or replace the
	// existing value with.
	baseValue string

	// basePresent reports whether the apex itself carries an xml:base
	// attribute.
	basePresent bool
}

// buildApexFix computes the document-subset fixups for a subtree apex:
// inheritance of xml:lang/xml:space from omitted ancestors and xml:base path
// fixup across omitted ancestors. xml:id is deliberately not handled.
func buildApexFix(apex *Element, chain []*Element) (*apexFix, error) {
	fix := &apexFix{}

	// Simple inheritable attributes in the xml namespace: inherit the
	// nearest occurrence from an omitted ancestor unless the apex already
	// declares the attribute itself.
	for _, key := range []string{"lang", "space"} {
		if hasXMLAttr(apex, key) {
			continue
		}
		for i := len(chain) - 1; i >= 0; i-- {
			if value, ok := findXMLAttr(chain[i], key); ok {
				if fix.inherit == nil {
					fix.inherit = make(map[string]string)
				}
				fix.inherit[key] = value
				break
			}
		}
	}

	// xml:base fixup. Only performed when at least one omitted ancestor
	// carries an xml:base attribute. Values are gathered from outermost to
	// innermost, with the apex's own value (if present) last.
	var values []string
	ancestorBase := false
	for _, a := range chain {
		if value, ok := findXMLAttr(a, "base"); ok {
			values = append(values, value)
			ancestorBase = true
		}
	}
	if value, ok := findXMLAttr(apex, "base"); ok {
		fix.basePresent = true
		values = append(values, value)
	}
	if ancestorBase {
		result := values[0]
		for i := 1; i < len(values); i++ {
			result = joinURIReferences(result, values[i])
		}
		if result != "" {
			fix.baseRender = true
			fix.baseValue = result
		}
	}

	return fix, nil
}

// writeElement writes the canonical form of an element and, for elements in
// the rendered node-set, all of its child tokens. parentScope is the
// complete in-scope namespace context of the element's nearest rendered
// ancestor; apex is true for a document element or subtree root, which has no
// rendered ancestor.
func (c *canonicalizer) writeElement(e *Element, parentScope map[string]string, apex bool, fix *apexFix) error {
	scope, err := cloneScopeWithDecls(parentScope, e)
	if err != nil {
		return err
	}

	qname := e.FullTag()
	c.buf.WriteByte('<')
	c.buf.WriteString(qname)

	// Namespace axis. Collect the namespace nodes rendered on this element.
	type nsNode struct {
		prefix string
		uri    string
	}
	var nodes []nsNode

	if apex {
		// The apex has no rendered ancestor: every in-scope namespace node
		// is a candidate for rendering. This includes bindings inherited
		// from omitted ancestors.
		for prefix, uri := range scope {
			if prefix == "xml" && uri == xmlNamespaceURI {
				continue
			}
			nodes = append(nodes, nsNode{prefix, uri})
		}
	} else {
		// A non-apex element renders only its own declarations, and only
		// when the nearest rendered ancestor does not have an equivalent
		// node in scope.
		for _, a := range e.Attr {
			prefix, ok := namespaceDeclPrefix(a)
			if !ok || a.Value == "" {
				continue
			}
			if parentScope[prefix] == a.Value {
				continue
			}
			if prefix == "xml" && a.Value == xmlNamespaceURI {
				continue
			}
			nodes = append(nodes, nsNode{prefix, a.Value})
		}
	}

	// A forced empty default namespace declaration is rendered when this
	// element has no default namespace node but its nearest rendered
	// ancestor has a non-empty one. An apex has no rendered ancestor, so it
	// never receives a forced declaration.
	_, hasDefault := scope[""]
	_, parentDefault := parentScope[""]
	forceDefault := !hasDefault && parentDefault && !apex

	slices.SortFunc(nodes, func(a, b nsNode) int {
		return strings.Compare(a.prefix, b.prefix)
	})

	if forceDefault {
		c.writeNamespace("", "")
	}
	for _, n := range nodes {
		c.writeNamespace(n.prefix, n.uri)
	}

	// Attribute axis.
	type attrNode struct {
		qname string
		key   string
		uri   string
		value string
	}
	var attrs []attrNode

	baseSuppressed := fix != nil && fix.basePresent && !fix.baseRender
	for _, a := range e.Attr {
		if _, isNS := namespaceDeclPrefix(a); isNS {
			continue
		}
		if a.Space == "xml" && a.Key == "base" && baseSuppressed {
			continue
		}

		value := a.Value
		if a.Space == "xml" && a.Key == "base" && fix != nil && fix.baseRender {
			value = fix.baseValue
		}

		uri := ""
		if a.Space != "" {
			uri = scope[a.Space]
		}
		attrs = append(attrs, attrNode{
			qname: a.FullKey(),
			key:   a.Key,
			uri:   uri,
			value: value,
		})
	}

	// Document-subset attribute fixups at the apex.
	if fix != nil {
		if !fix.basePresent && fix.baseRender {
			attrs = append(attrs, attrNode{
				qname: "xml:base",
				key:   "base",
				uri:   xmlNamespaceURI,
				value: fix.baseValue,
			})
		}
		for _, key := range []string{"lang", "space"} {
			if value, ok := fix.inherit[key]; ok {
				attrs = append(attrs, attrNode{
					qname: "xml:" + key,
					key:   key,
					uri:   xmlNamespaceURI,
					value: value,
				})
			}
		}
	}

	slices.SortStableFunc(attrs, func(a, b attrNode) int {
		if v := strings.Compare(a.uri, b.uri); v != 0 {
			return v
		}
		return strings.Compare(a.key, b.key)
	})

	for _, a := range attrs {
		c.writeAttribute(a.qname, a.value)
	}

	if len(e.Child) == 0 {
		// Explicit start-end tag pair, even for empty elements.
		c.buf.WriteString("></")
		c.buf.WriteString(qname)
		c.buf.WriteByte('>')
		return nil
	}

	c.buf.WriteByte('>')
	for _, tok := range e.Child {
		switch t := tok.(type) {
		case *Element:
			if err := c.writeElement(t, scope, false, nil); err != nil {
				return err
			}
		case *CharData:
			escapeString(&c.buf, t.Data, escapeCanonicalText)
		case *Comment:
			if c.comments {
				c.writeComment(t)
			}
		case *ProcInst:
			c.writeProcInst(t)
		case *Directive:
			// Directives (including the document type declaration) are
			// never rendered.
		}
	}
	c.buf.WriteString("</")
	c.buf.WriteString(qname)
	c.buf.WriteByte('>')
	return nil
}

// writeNamespace writes a single canonical namespace declaration. An empty
// prefix renders the default namespace declaration.
func (c *canonicalizer) writeNamespace(prefix, uri string) {
	c.buf.WriteByte(' ')
	if prefix == "" {
		c.buf.WriteString("xmlns=")
	} else {
		c.buf.WriteString("xmlns:")
		c.buf.WriteString(prefix)
		c.buf.WriteByte('=')
	}
	c.buf.WriteByte('"')
	escapeString(&c.buf, uri, escapeCanonicalAttr)
	c.buf.WriteByte('"')
}

// writeAttribute writes a single canonical attribute.
func (c *canonicalizer) writeAttribute(qname, value string) {
	c.buf.WriteByte(' ')
	c.buf.WriteString(qname)
	c.buf.WriteString(`="`)
	escapeString(&c.buf, value, escapeCanonicalAttr)
	c.buf.WriteByte('"')
}

// writeComment writes a canonical comment. Line breaks are normalized to #xA
// to compensate for input parsers that retain carriage returns in comments.
func (c *canonicalizer) writeComment(cm *Comment) {
	c.buf.WriteString("<!--")
	c.buf.WriteString(normalizeLineBreaks(cm.Data))
	c.buf.WriteString("-->")
}

// writeProcInst writes a canonical processing instruction. A single space
// separates the target from non-empty instruction text, and line breaks are
// normalized to #xA.
func (c *canonicalizer) writeProcInst(p *ProcInst) {
	c.buf.WriteString("<?")
	c.buf.WriteString(p.Target)
	if p.Inst != "" {
		c.buf.WriteByte(' ')
		c.buf.WriteString(normalizeLineBreaks(p.Inst))
	}
	c.buf.WriteString("?>")
}

// namespaceDeclPrefix returns the prefix declared by an attribute if it is a
// namespace declaration. An empty prefix is returned for a default namespace
// declaration (xmlns="...").
func namespaceDeclPrefix(a Attr) (string, bool) {
	if a.Space == "xmlns" {
		return a.Key, true
	}
	if a.Space == "" && a.Key == "xmlns" {
		return "", true
	}
	return "", false
}

// applyNamespaceDecls updates scope with all namespace declarations carried
// by element e. An error is returned when a non-empty namespace name is not
// an absolute URI reference.
func applyNamespaceDecls(e *Element, scope map[string]string) error {
	for _, a := range e.Attr {
		prefix, ok := namespaceDeclPrefix(a)
		if !ok {
			continue
		}
		if a.Value == "" {
			delete(scope, prefix)
			continue
		}
		if !isAbsoluteURI(a.Value) {
			return errRelativeNamespaceURI
		}
		scope[prefix] = a.Value
	}
	return nil
}

// cloneScopeWithDecls returns a copy of scope updated with element e's own
// namespace declarations.
func cloneScopeWithDecls(scope map[string]string, e *Element) (map[string]string, error) {
	clone := make(map[string]string, len(scope)+4)
	for prefix, uri := range scope {
		clone[prefix] = uri
	}
	if err := applyNamespaceDecls(e, clone); err != nil {
		return nil, err
	}
	return clone, nil
}

// hasXMLAttr reports whether e carries an attribute in the xml namespace
// with the given local name.
func hasXMLAttr(e *Element, local string) bool {
	_, ok := findXMLAttr(e, local)
	return ok
}

// findXMLAttr returns the value of the xml-namespaced attribute with the
// given local name carried directly by e.
func findXMLAttr(e *Element, local string) (string, bool) {
	for _, a := range e.Attr {
		if a.Space == "xml" && a.Key == local {
			return a.Value, true
		}
	}
	return "", false
}

// normalizeLineBreaks converts the two-character #xD#xA sequence and the
// standalone #xD character to #xA, as required during input line break
// normalization.
func normalizeLineBreaks(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// isAbsoluteURI reports whether s is an absolute URI reference, i.e. one
// beginning with a scheme component per RFC 3986. The empty string (used to
// undeclare the default namespace) is not itself a namespace name and is
// handled by callers.
func isAbsoluteURI(s string) bool {
	i := strings.IndexByte(s, ':')
	if i <= 0 {
		return false
	}
	// Everything before the colon must be a valid scheme, which implies no
	// '/', '?' or '#' may precede the colon.
	for j := 0; j < i; j++ {
		c := s[j]
		switch {
		case j == 0:
			if !isAlphaByte(c) {
				return false
			}
		case !isAlphaByte(c) && !isDigitByte(c) && c != '+' && c != '-' && c != '.':
			return false
		}
	}
	return true
}

func isAlphaByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isDigitByte(c byte) bool {
	return c >= '0' && c <= '9'
}

// -------------------------------------------------------------------
// xml:base fixup: a restricted implementation of the RFC 3986 URI
// reference join, with the Canonical XML 1.1 modifications.
// -------------------------------------------------------------------

// parsedURI holds the parsed components of a URI reference.
type parsedURI struct {
	scheme    string
	hasScheme bool
	authority string
	hasAuth   bool
	path      string
	query     string
	hasQuery  bool
}

// parseURIReference parses a URI reference per RFC 3986 appendix B, with the
// fragment component always discarded (Canonical XML ignores it).
func parseURIReference(s string) parsedURI {
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = s[:i]
	}

	var p parsedURI
	rest := s

	// Extract the scheme only if the colon precedes any '/', '?' or '#' and
	// the prefix is a valid scheme.
	if colon := strings.IndexByte(rest, ':'); colon > 0 {
		valid := true
		for j := 0; j < colon; j++ {
			c := rest[j]
			if c == '/' || c == '?' || c == '#' {
				valid = false
				break
			}
		}
		if valid && isScheme(rest[:colon]) {
			p.scheme = rest[:colon]
			p.hasScheme = true
			rest = rest[colon+1:]
		}
	}

	if strings.HasPrefix(rest, "//") {
		rest = rest[2:]
		end := len(rest)
		if i := strings.IndexAny(rest, "/?"); i >= 0 {
			end = i
		}
		p.authority = rest[:end]
		p.hasAuth = true
		rest = rest[end:]
	}

	if i := strings.IndexByte(rest, '?'); i >= 0 {
		p.path = rest[:i]
		p.query = rest[i+1:]
		p.hasQuery = true
	} else {
		p.path = rest
	}
	return p
}

// isScheme reports whether s is a valid RFC 3986 scheme.
func isScheme(s string) bool {
	if len(s) == 0 || !isAlphaByte(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !isAlphaByte(c) && !isDigitByte(c) && c != '+' && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

// joinURIReferences resolves the URI reference r against the base URI b,
// following RFC 3986 section 5.2.2 with the modifications mandated by the
// Canonical XML 1.1 specification.
func joinURIReferences(b, r string) string {
	base := parseURIReference(b)
	ref := parseURIReference(r)

	// Replace a trailing ".." segment of the base path with "../" before
	// processing.
	if base.path == ".." {
		base.path = "../"
	} else if strings.HasSuffix(base.path, "/..") {
		base.path += "/"
	}

	var t parsedURI
	switch {
	case ref.hasScheme:
		t.scheme, t.hasScheme = ref.scheme, true
		t.authority, t.hasAuth = ref.authority, ref.hasAuth
		t.path = removeDotSegments(ref.path, ref.hasAuth)
		t.query, t.hasQuery = ref.query, ref.hasQuery

	case ref.hasAuth:
		t.authority, t.hasAuth = ref.authority, true
		t.path = removeDotSegments(ref.path, true)
		t.query, t.hasQuery = ref.query, ref.hasQuery

	case ref.path == "":
		t.scheme, t.hasScheme = base.scheme, base.hasScheme
		t.authority, t.hasAuth = base.authority, base.hasAuth
		t.path = base.path
		if ref.hasQuery {
			t.query, t.hasQuery = ref.query, true
		} else {
			t.query, t.hasQuery = base.query, base.hasQuery
		}

	case ref.path[0] == '/':
		t.scheme, t.hasScheme = base.scheme, base.hasScheme
		t.authority, t.hasAuth = base.authority, base.hasAuth
		t.path = removeDotSegments(ref.path, true)
		t.query, t.hasQuery = ref.query, ref.hasQuery

	default:
		t.scheme, t.hasScheme = base.scheme, base.hasScheme
		t.authority, t.hasAuth = base.authority, base.hasAuth
		t.path = removeDotSegments(mergePaths(base, ref.path), base.hasAuth)
		t.query, t.hasQuery = ref.query, ref.hasQuery
	}

	var sb strings.Builder
	if t.hasScheme {
		sb.WriteString(t.scheme)
		sb.WriteByte(':')
	}
	if t.hasAuth {
		sb.WriteString("//")
		sb.WriteString(t.authority)
	}
	sb.WriteString(t.path)
	if t.hasQuery {
		sb.WriteByte('?')
		sb.WriteString(t.query)
	}
	return sb.String()
}

// mergePaths implements RFC 3986 section 5.2.3: merge a relative reference
// path with the base URI's path.
func mergePaths(base parsedURI, refPath string) string {
	if base.hasAuth && base.path == "" {
		return "/" + refPath
	}
	if i := strings.LastIndexByte(base.path, '/'); i >= 0 {
		return base.path[:i+1] + refPath
	}
	return refPath
}

// removeDotSegments implements RFC 3986 section 5.2.4 with the Canonical XML
// 1.1 modifications: leading "../" segments are retained for relative paths,
// consecutive slashes are collapsed, and a trailing ".." segment gains a
// final slash.
func removeDotSegments(path string, absolute bool) string {
	if path == "" {
		return ""
	}

	// Collapse runs of consecutive "/" characters.
	var sb strings.Builder
	prevSlash := false
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			if prevSlash {
				continue
			}
			prevSlash = true
		} else {
			prevSlash = false
		}
		sb.WriteByte(path[i])
	}
	collapsed := sb.String()
	trailingSlash := strings.HasSuffix(collapsed, "/")

	var out []string
	leading := 0
	segments := strings.Split(collapsed, "/")
	for i, seg := range segments {
		if i == len(segments)-1 && seg == "" {
			continue // marker for the trailing slash
		}
		switch seg {
		case ".":
			// Removed.
		case "..":
			switch {
			case len(out) > 0:
				out = out[:len(out)-1]
			case absolute:
				// Popping at the root of an absolute path has no effect.
			default:
				leading++
			}
		case "":
			// Empty interior segment (e.g. the leading segment of an
			// absolute path); nothing to add.
		default:
			out = append(out, seg)
		}
	}

	var result strings.Builder
	if absolute {
		result.WriteByte('/')
	}
	result.WriteString(strings.Repeat("../", leading))
	result.WriteString(strings.Join(out, "/"))
	res := result.String()

	if strings.HasSuffix(path, "..") {
		// Append a slash to a surviving trailing ".." segment, but avoid
		// turning an empty relative result into an absolute-looking path.
		if res != "" && !strings.HasSuffix(res, "/") {
			res += "/"
		}
	} else if trailingSlash && res != "" && !strings.HasSuffix(res, "/") {
		res += "/"
	}
	return res
}
