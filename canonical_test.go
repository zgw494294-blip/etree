// Copyright 2025 Brett Vickers.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etree_test

import (
	"strings"
	"testing"

	"github.com/beevik/etree"
)

// contractCanonicalDocument mechanically bridges external acceptance tests
// to the public W3C Canonical XML 1.1 document API. It must not implement,
// correct, or post-process canonicalization itself.
func contractCanonicalDocument(input *etree.Document, comments bool) ([]byte, error) {
	return input.CanonicalXML(comments)
}

// contractCanonicalSubtree mechanically bridges external acceptance tests to
// the public W3C Canonical XML 1.1 subtree API. It must not implement,
// correct, or post-process canonicalization itself.
func contractCanonicalSubtree(input *etree.Element, comments bool) ([]byte, error) {
	return input.CanonicalXML(comments)
}

func mustParseDoc(t *testing.T, s string) *etree.Document {
	t.Helper()
	doc := etree.NewDocument()
	if err := doc.ReadFromString(s); err != nil {
		t.Fatalf("failed to parse XML: %v\n%s", err, s)
	}
	return doc
}

func checkCanonical(t *testing.T, got []byte, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected canonicalization error: %v", err)
	}
	if string(got) != want {
		t.Errorf("canonical output mismatch.\nGot:  %q\nWant: %q", string(got), want)
	}
}

// TestCanonicalXMLW3CStyle constructs test XML and its canonical output.
func TestCanonicalXMLDocumentPIsComments(t *testing.T) {
	src := `<?xml version="1.0"?>
<?xml-stylesheet href="doc.xsl" type="text/xsl" ?>
<!DOCTYPE doc SYSTEM "doc.dtd">
<doc>Hello, world!<!-- Comment 1 --></doc>
<?pi-without-data ?>
<!-- Comment 2 -->
<!-- Comment 3 -->
`
	doc := mustParseDoc(t, src)

	withoutComments := `<?xml-stylesheet href="doc.xsl" type="text/xsl" ?>` + "\n" +
		`<doc>Hello, world!</doc>` + "\n" +
		`<?pi-without-data?>`
	got, err := contractCanonicalDocument(doc, false)
	checkCanonical(t, got, err, withoutComments)

	withComments := `<?xml-stylesheet href="doc.xsl" type="text/xsl" ?>` + "\n" +
		`<doc>Hello, world!<!-- Comment 1 --></doc>` + "\n" +
		`<?pi-without-data?>` + "\n" +
		`<!-- Comment 2 -->` + "\n" +
		`<!-- Comment 3 -->`
	got, err = contractCanonicalDocument(doc, true)
	checkCanonical(t, got, err, withComments)
}

func TestCanonicalXMLRootPIsPlacement(t *testing.T) {
	doc := mustParseDoc(t, `<?a 1?><r></r><?b 2?>`)
	got, err := contractCanonicalDocument(doc, true)
	checkCanonical(t, got, err, "<?a 1?>\n<r></r>\n<?b 2?>")

	doc = mustParseDoc(t, `<?a?><?b?><r/>`)
	got, err = contractCanonicalDocument(doc, true)
	checkCanonical(t, got, err, "<?a?>\n<?b?>\n<r></r>")

	doc = mustParseDoc(t, `<r/><?a?><?b?>`)
	got, err = contractCanonicalDocument(doc, true)
	checkCanonical(t, got, err, "<r></r>\n<?a?>\n<?b?>")
}

func TestCanonicalXMLNamespaces(t *testing.T) {
	src := `<doc>
  <e5 a:attr="out" b:attr="sorted" attr2="all" attr="I'm"
      xmlns:b="http://www.ietf.org"
      xmlns:a="http://www.w3.org"
      xmlns="http://example.org"/>
  <e6 xmlns="" xmlns:a="http://www.w3.org">
    <e7 xmlns="http://www.ietf.org">
      <e8 xmlns="" xmlns:a="http://www.w3.org">
        <e9 xmlns="" xmlns:a="http://www.ietf.org"/>
      </e8>
    </e7>
  </e6>
</doc>`
	want := `<doc>
  <e5 xmlns="http://example.org" xmlns:a="http://www.w3.org" xmlns:b="http://www.ietf.org" attr="I'm" attr2="all" b:attr="sorted" a:attr="out"></e5>
  <e6 xmlns:a="http://www.w3.org">
    <e7 xmlns="http://www.ietf.org">
      <e8 xmlns="">
        <e9 xmlns:a="http://www.ietf.org"></e9>
      </e8>
    </e7>
  </e6>
</doc>`
	doc := mustParseDoc(t, src)
	got, err := contractCanonicalDocument(doc, false)
	checkCanonical(t, got, err, want)
}

func TestCanonicalXMLAttributeOrdering(t *testing.T) {
	doc := mustParseDoc(t, `<e xmlns:a="http://www.w3.org" xmlns:b="http://www.ietf.org" a:x="1" b:y="2" z="3"/>`)
	got, err := contractCanonicalDocument(doc, false)
	checkCanonical(t, got, err,
		`<e xmlns:a="http://www.w3.org" xmlns:b="http://www.ietf.org" z="3" b:y="2" a:x="1"></e>`)

	// An unqualified attribute never joins the default namespace: it sorts
	// ahead of namespaced attributes regardless of the default namespace.
	doc = mustParseDoc(t, `<e xmlns="urn:default" plain="v" xmlns:p="urn:p" p:q="w"/>`)
	got, err = contractCanonicalDocument(doc, false)
	checkCanonical(t, got, err,
		`<e xmlns="urn:default" xmlns:p="urn:p" plain="v" p:q="w"></e>`)
}

func TestCanonicalXMLCharsAndCDATA(t *testing.T) {
	src := `<doc>
  <text>First line&#x0d;&#10;Second line</text>
  <value>&#x32;</value>
  <compute><![CDATA[value>"0" && value<"10" ?"valid":"error"]]></compute>
  <compute expr='value>"0" &amp;&amp; value&lt;"10" ?"valid":"error"'>valid</compute>
  <norm attr=' &apos; &#x20;&#13;&#xa;&#9; &apos; '/>
</doc>`
	want := `<doc>
  <text>First line&#xD;
Second line</text>
  <value>2</value>
  <compute>value&gt;"0" &amp;&amp; value&lt;"10" ?"valid":"error"</compute>
  <compute expr="value>&quot;0&quot; &amp;&amp; value&lt;&quot;10&quot; ?&quot;valid&quot;:&quot;error&quot;">valid</compute>
  <norm attr=" '  &#xD;&#xA;&#x9; ' "></norm>
</doc>`
	doc := mustParseDoc(t, src)
	got, err := contractCanonicalDocument(doc, false)
	checkCanonical(t, got, err, want)

	// Standalone CDATA is emitted as escaped text only.
	doc = mustParseDoc(t, `<a><![CDATA[<x & y>]]></a>`)
	got, err = contractCanonicalDocument(doc, false)
	checkCanonical(t, got, err, `<a>&lt;x &amp; y&gt;</a>`)
}

func TestCanonicalXMLUnicodeUTF8(t *testing.T) {
	doc := mustParseDoc(t, `<doc>© ☃</doc>`)
	got, err := contractCanonicalDocument(doc, false)
	checkCanonical(t, got, err, `<doc>© ☃</doc>`)
	if !strings.HasPrefix(string(got), "<doc>") {
		t.Fatal("canonical output must not contain a BOM")
	}
}

func TestCanonicalXMLSubtreeInheritedNamespaces(t *testing.T) {
	src := `<doc xmlns:n0="http://www.w3.org" xmlns:n1="http://www.example.com">` +
		`<n0:elem11><n0:elem12><n1:elem21><n1:elem22></n1:elem22>` +
		`</n1:elem21></n0:elem12></n0:elem11></doc>`
	doc := mustParseDoc(t, src)
	e21 := doc.FindElement("//n1:elem21")
	if e21 == nil {
		t.Fatal("elem21 not found")
	}
	got, err := contractCanonicalSubtree(e21, false)
	checkCanonical(t, got, err,
		`<n1:elem21 xmlns:n0="http://www.w3.org" xmlns:n1="http://www.example.com">`+
			`<n1:elem22></n1:elem22></n1:elem21>`)

	// Empty default namespace propagates from an omitted ancestor.
	doc = mustParseDoc(t, `<doc xmlns="http://www.ietf.org"><e1><e2 xmlns=""><e3 id="E3"/></e2></e1></doc>`)
	e3 := doc.FindElement("//e3")
	got, err = contractCanonicalSubtree(e3, false)
	checkCanonical(t, got, err, `<e3 id="E3"></e3>`)

	// A full subtree containing the un-declaring element keeps that
	// declaration on it.
	e1 := doc.FindElement("//e1")
	got, err = contractCanonicalSubtree(e1, false)
	checkCanonical(t, got, err,
		`<e1 xmlns="http://www.ietf.org"><e2 xmlns=""><e3 id="E3"></e3></e2></e1>`)
}

func TestCanonicalXMLSubtreeXMLAttrs(t *testing.T) {
	src := `<doc xmlns="http://www.ietf.org" xml:base="something/else" xml:lang="fr">` +
		`<e1 xml:lang="en"><e2 xml:id="abc" xml:base="bar/"><e3 id="E3" xml:base="foo"/>` +
		`</e2></e1></doc>`
	doc := mustParseDoc(t, src)

	// Apex e3 absorbs bases from the omitted ancestor chain, inherits the
	// nearest xml:lang, never inherits xml:id, and re-emits the inherited
	// default namespace.
	e3 := doc.FindElement("//e3")
	got, err := contractCanonicalSubtree(e3, false)
	checkCanonical(t, got, err,
		`<e3 xmlns="http://www.ietf.org" id="E3" xml:base="something/bar/foo" xml:lang="en"></e3>`)

	// e2 is included and carries its own xml:id and fixed base; the child's
	// relative base is left untouched and it needs no redundant namespace
	// declaration once its parent is rendered.
	e2 := doc.FindElement("//e2")
	got, err = contractCanonicalSubtree(e2, false)
	checkCanonical(t, got, err,
		`<e2 xmlns="http://www.ietf.org" xml:base="something/bar/" xml:id="abc" xml:lang="en">`+
			`<e3 id="E3" xml:base="foo"></e3></e2>`)

	// Apex without its own xml:base gets a synthesized one.
	doc = mustParseDoc(t, `<a xml:base="base/"><b></b></a>`)
	b := doc.FindElement("//b")
	got, err = contractCanonicalSubtree(b, false)
	checkCanonical(t, got, err, `<b xml:base="base/"></b>`)

	// A fixup that resolves to the empty string suppresses xml:base.
	doc = mustParseDoc(t, `<a xml:base="abc/"><b xml:base="../"></b></a>`)
	b = doc.FindElement("//b")
	got, err = contractCanonicalSubtree(b, false)
	checkCanonical(t, got, err, `<b></b>`)

	// xml:space is inherited; an apex declaration suppresses inheritance.
	doc = mustParseDoc(t, `<a xml:space="preserve"><b xml:space="default"><c/></b></a>`)
	c := doc.FindElement("//c")
	got, err = contractCanonicalSubtree(c, false)
	checkCanonical(t, got, err, `<c xml:space="default"></c>`)
	b = doc.FindElement("//b")
	got, err = contractCanonicalSubtree(b, false)
	checkCanonical(t, got, err, `<b xml:space="default"><c></c></b>`)
}

func TestCanonicalXMLBaseDotSegments(t *testing.T) {
	// With the complete ancestor chain omitted, the fixed base of the apex
	// is the RFC 3986 join of every ancestor base: foo/bar + ../ + ../ + x
	// resolves to ../x (leading ".." survives because the path stays
	// relative).
	src := `<a xml:base="foo/bar"><b xml:base=".."><c xml:base=".."><d xml:base="x"></d></c></b></a>`
	doc := mustParseDoc(t, src)
	d := doc.FindElement("//d")
	got, err := contractCanonicalSubtree(d, false)
	checkCanonical(t, got, err, `<d xml:base="../x"></d>`)

	// Leading "../" segments are retained, and a trailing ".." gains a
	// final slash.
	doc = mustParseDoc(t, `<a xml:base="../"><b xml:base="../"></b></a>`)
	b := doc.FindElement("//b")
	got, err = contractCanonicalSubtree(b, false)
	checkCanonical(t, got, err, `<b xml:base="../../"></b>`)

	doc = mustParseDoc(t, `<a xml:base=".."><b xml:base=".."></b></a>`)
	b = doc.FindElement("//b")
	got, err = contractCanonicalSubtree(b, false)
	checkCanonical(t, got, err, `<b xml:base="../../"></b>`)

	// Absolute authority is preserved once a base with a scheme appears.
	doc = mustParseDoc(t, `<a xml:base="http://example.org/x/y"><b xml:base="../z"></b></a>`)
	b = doc.FindElement("//b")
	got, err = contractCanonicalSubtree(b, false)
	checkCanonical(t, got, err, `<b xml:base="http://example.org/z"></b>`)

	// Query and fragment handling: fragments are discarded.
	doc = mustParseDoc(t, `<a xml:base="a/b/"><b xml:base="c?x=1#frag"></b></a>`)
	b = doc.FindElement("//b")
	got, err = contractCanonicalSubtree(b, false)
	checkCanonical(t, got, err, `<b xml:base="a/b/c?x=1"></b>`)
}

func TestCanonicalXMLRelativeNamespaceURIError(t *testing.T) {
	doc := mustParseDoc(t, `<doc xmlns:rel="relative/uri"><rel:x></rel:x></doc>`)
	if b, err := contractCanonicalDocument(doc, false); err == nil {
		t.Errorf("expected error for relative prefixed namespace URI, got %q", b)
	}

	e := doc.FindElement("//rel:x")
	if b, err := contractCanonicalSubtree(e, false); err == nil {
		t.Errorf("expected error for relative prefixed namespace URI in subtree, got %q", b)
	}

	doc = mustParseDoc(t, `<doc xmlns="relative-default"></doc>`)
	if b, err := contractCanonicalDocument(doc, false); err == nil {
		t.Errorf("expected error for relative default namespace URI, got %q", b)
	}

	// A subsequent independent call on a valid tree still succeeds.
	good := mustParseDoc(t, `<doc xmlns:ok="urn:ok"><ok:x/></doc>`)
	got, err := contractCanonicalDocument(good, false)
	checkCanonical(t, got, err, `<doc xmlns:ok="urn:ok"><ok:x></ok:x></doc>`)

	// Absolute URIs of various schemes are accepted.
	good = mustParseDoc(t, `<doc xmlns:a="urn:isbn:123" xmlns:b="https://example.org/x"/>`)
	got, err = contractCanonicalDocument(good, false)
	checkCanonical(t, got, err,
		`<doc xmlns:a="urn:isbn:123" xmlns:b="https://example.org/x"></doc>`)
}

func TestCanonicalXMLDoesNotMutateTree(t *testing.T) {
	src := `<doc xmlns:z="urn:z" b="2" a="1" z:x="3"><child>text<!-- c --></child></doc>`
	doc := mustParseDoc(t, src)
	before, err := doc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	beforeSettings := doc.WriteSettings

	if _, err = contractCanonicalDocument(doc, true); err != nil {
		t.Fatal(err)
	}
	if _, err = contractCanonicalDocument(doc, false); err != nil {
		t.Fatal(err)
	}
	child := doc.FindElement("//child")
	if _, err = contractCanonicalSubtree(child, true); err != nil {
		t.Fatal(err)
	}
	if _, err = contractCanonicalSubtree(child, false); err != nil {
		t.Fatal(err)
	}

	after, err := doc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Errorf("tree was modified by canonicalization.\nBefore: %q\nAfter:  %q", before, after)
	}
	if doc.WriteSettings != beforeSettings {
		t.Error("WriteSettings were modified by canonicalization")
	}

	// The root element's subtree serialized directly matches the document
	// canonical output for the tree's body.
	root := doc.Root()
	sub, err := contractCanonicalSubtree(root, true)
	if err != nil {
		t.Fatal(err)
	}
	full, err := contractCanonicalDocument(doc, true)
	if err != nil {
		t.Fatal(err)
	}
	if string(sub) != string(full) {
		t.Errorf("root subtree mismatch.\nsub:  %q\nfull: %q", sub, full)
	}
}

func TestCanonicalXMLIndependentCallsIsolated(t *testing.T) {
	doc1 := mustParseDoc(t, `<a xmlns="urn:a"/>`)
	doc2 := mustParseDoc(t, `<b xmlns="urn:b"/>`)

	g1, err := contractCanonicalDocument(doc1, false)
	if err != nil {
		t.Fatal(err)
	}
	g2, err := contractCanonicalDocument(doc2, true)
	if err != nil {
		t.Fatal(err)
	}
	g1again, err := contractCanonicalDocument(doc1, true)
	if err != nil {
		t.Fatal(err)
	}
	checkCanonical(t, g1, err, `<a xmlns="urn:a"></a>`)
	if string(g2) != `<b xmlns="urn:b"></b>` {
		t.Errorf("got %q", g2)
	}
	if string(g1again) != `<a xmlns="urn:a"></a>` {
		t.Errorf("got %q", g1again)
	}
}

func TestCanonicalXMLCommentsInsideElements(t *testing.T) {
	doc := mustParseDoc(t, "<a>before<!--c1\r\nc2-->after<?p d?></a>")
	got, err := contractCanonicalDocument(doc, true)
	checkCanonical(t, got, err, "<a>before<!--c1\nc2-->after<?p d?></a>")

	got, err = contractCanonicalDocument(doc, false)
	checkCanonical(t, got, err, "<a>beforeafter<?p d?></a>")
}

func TestCanonicalXMLIdempotent(t *testing.T) {
	cases := []string{
		`<?p?><doc xmlns="urn:x" xmlns:a="urn:a" a:v="1"><!-- hi --><n a:w="2"></n></doc><?q?>`,
		`<doc>text&amp;more</doc>`,
		`<doc xmlns:p="urn:p"><p:e p:attr="v">   </p:e></doc>`,
	}
	for _, src := range cases {
		doc := mustParseDoc(t, src)
		first, err := contractCanonicalDocument(doc, true)
		if err != nil {
			t.Fatal(err)
		}
		doc2 := mustParseDoc(t, string(first))
		second, err := contractCanonicalDocument(doc2, true)
		if err != nil {
			t.Fatal(err)
		}
		if string(first) != string(second) {
			t.Errorf("canonical form not idempotent.\nfirst:  %q\nsecond: %q", first, second)
		}
	}
}

func TestCanonicalXMLExplicitEndTags(t *testing.T) {
	doc := mustParseDoc(t, `<a><b/><c></c><d> </d></a>`)
	got, err := contractCanonicalDocument(doc, false)
	checkCanonical(t, got, err, `<a><b></b><c></c><d> </d></a>`)
}

func TestCanonicalXMLPreserveCDataIsText(t *testing.T) {
	doc := etree.NewDocument()
	doc.ReadSettings.PreserveCData = true
	if err := doc.ReadFromString(`<a><![CDATA[x < y & z]]></a>`); err != nil {
		t.Fatal(err)
	}
	got, err := contractCanonicalDocument(doc, false)
	checkCanonical(t, got, err, `<a>x &lt; y &amp; z</a>`)
}

func TestCanonicalXMLInheritedPrefixedAttribute(t *testing.T) {
	doc := mustParseDoc(t, `<a xmlns:p="urn:p"><b><c p:x="1"/></b></a>`)
	c := doc.FindElement("//c")
	got, err := contractCanonicalSubtree(c, false)
	checkCanonical(t, got, err, `<c xmlns:p="urn:p" p:x="1"></c>`)
}

func TestCanonicalXMLRedundantDeclarationChain(t *testing.T) {
	doc := mustParseDoc(t, `<a xmlns:p="urn:u"><b xmlns:p="urn:u"><c xmlns:p="urn:u2"></c></b></a>`)
	got, err := contractCanonicalDocument(doc, false)
	checkCanonical(t, got, err,
		`<a xmlns:p="urn:u"><b><c xmlns:p="urn:u2"></c></b></a>`)

	c := doc.FindElement("//c")
	got, err = contractCanonicalSubtree(c, false)
	checkCanonical(t, got, err, `<c xmlns:p="urn:u2"></c>`)

	b := doc.FindElement("//b")
	got, err = contractCanonicalSubtree(b, false)
	checkCanonical(t, got, err,
		`<b xmlns:p="urn:u"><c xmlns:p="urn:u2"></c></b>`)
}

func TestCanonicalXMLSubtreeCommentsToggle(t *testing.T) {
	doc := mustParseDoc(t, `<a><b><!--keep--><c/></b></a>`)
	b := doc.FindElement("//b")
	got, err := contractCanonicalSubtree(b, true)
	checkCanonical(t, got, err, `<b><!--keep--><c></c></b>`)
	got, err = contractCanonicalSubtree(b, false)
	checkCanonical(t, got, err, `<b><c></c></b>`)
}

func TestCanonicalXMLNormalWriteSettingsUnaffected(t *testing.T) {
	doc := mustParseDoc(t, `<a><b/></a>`)
	if _, err := contractCanonicalDocument(doc, true); err != nil {
		t.Fatal(err)
	}
	// Normal serialization still uses its own WriteSettings and is
	// unaffected by canonicalization.
	s, err := doc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	if s != `<a><b/></a>` {
		t.Errorf("unexpected normal write output: %q", s)
	}
}
