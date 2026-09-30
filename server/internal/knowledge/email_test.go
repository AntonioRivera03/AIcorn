package knowledge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const multipartEmail = "From: =?UTF-8?Q?Fabiana_Garc=C3=ADa?= <fabiana@example.com>\r\n" +
	"To: Team <team@example.com>, ops@example.com\r\n" +
	"Subject: =?UTF-8?Q?Export_button_=E2=80=94_request?=\r\n" +
	"Date: Tue, 1 Sep 2026 09:30:00 -0400\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=outer\r\n" +
	"\r\n" +
	"--outer\r\n" +
	"Content-Type: multipart/alternative; boundary=inner\r\n" +
	"\r\n" +
	"--inner\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n" +
	"\r\n" +
	"Hi team,\r\n" +
	"\r\n" +
	"Could we add an export button? It=E2=80=99s urgent.\r\n" +
	"--inner\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n" +
	"\r\n" +
	"<p>Hi team,</p><p>HTML version</p>\r\n" +
	"--inner--\r\n" +
	"--outer\r\n" +
	"Content-Type: application/pdf\r\n" +
	"Content-Disposition: attachment; filename=spec.pdf\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	"JVBERi0xLjQK\r\n" +
	"--outer--\r\n"

func TestParseEmailPrefersPlainTextAndDecodesHeaders(t *testing.T) {
	e, err := parseEmail([]byte(multipartEmail))
	if err != nil {
		t.Fatal(err)
	}
	if e.Subject != "Export button — request" {
		t.Fatalf("subject %q", e.Subject)
	}
	if e.Details.From != "Fabiana García <fabiana@example.com>" || e.Details.To != "Team <team@example.com>, ops@example.com" {
		t.Fatalf("addresses %+v", e.Details)
	}
	if e.Details.Date != "2026-09-01T13:30:00Z" {
		t.Fatalf("date %q", e.Details.Date)
	}
	if e.Text != "Hi team,\n\nCould we add an export button? It’s urgent." {
		t.Fatalf("text %q", e.Text)
	}
}

func TestParseEmailFallsBackToHTML(t *testing.T) {
	raw := "From: a@example.com\r\nSubject: Hello\r\nContent-Type: text/html; charset=iso-8859-1\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		"PHN0eWxlPnB7Y29sb3I6cmVkfTwvc3R5bGU+PHA+Q2Fm6SAmYW1wOyB0ZWE8L3A+PHA+U2Vjb25kPGJyPmxpbmU8L3A+\r\n"
	e, err := parseEmail([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if e.Text != "Café & tea\nSecond\nline" {
		t.Fatalf("text %q", e.Text)
	}
}

func TestParseEmailRejectsOtherText(t *testing.T) {
	if _, err := parseEmail([]byte("just some notes\nwithout headers")); err == nil {
		t.Fatal("plain text accepted as an email")
	}
}

func TestUploadEmailUsesSubjectHeadersAndBody(t *testing.T) {
	s := testStore(t)
	d, err := s.UploadDocument(context.Background(), 101, "request.eml", []byte(multipartEmail))
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Export button — request" || d.File == nil || d.File.MediaType != "message/rfc822" {
		t.Fatalf("%+v", d)
	}
	if d.Details.Email == nil || d.Details.Email.From != "Fabiana García <fabiana@example.com>" {
		t.Fatalf("details %+v", d.Details)
	}
	if !strings.Contains(string(d.Body), "Could we add an export button?") || strings.Contains(string(d.Body), "HTML version") {
		t.Fatalf("body %s", d.Body)
	}
	// Details survive a round trip through the list.
	list, err := s.Documents(101)
	if err != nil || list[0].Details.Email == nil || list[0].Details.Email.Date == "" {
		t.Fatalf("%+v %v", list, err)
	}
}

type fakeMarkdown struct{ calls int }

func (f *fakeMarkdown) ToBody(_ context.Context, markdowns []string) ([]string, error) {
	f.calls++
	return []string{`[{"type":"h1","children":[{"text":"` + strings.TrimPrefix(markdowns[0], "# ") + `"}]}]`}, nil
}

func TestUploadTextKeepsLinesAndConvertsMarkdown(t *testing.T) {
	s := testStore(t)
	d, err := s.UploadDocument(context.Background(), 101, "notes.txt", []byte("first\n\nthird"))
	if err != nil {
		t.Fatal(err)
	}
	var nodes []map[string]any
	if err := json.Unmarshal(d.Body, &nodes); err != nil || len(nodes) != 3 {
		t.Fatalf("want a paragraph per line: %s %v", d.Body, err)
	}
	if d.Title != "notes" {
		t.Fatalf("title keeps the extension: %q", d.Title)
	}

	converter := &fakeMarkdown{}
	s.Markdown = converter
	d, err = s.UploadDocument(context.Background(), 101, "Plan.md", []byte("# Plan"))
	if err != nil || converter.calls != 1 || !strings.Contains(string(d.Body), `"h1"`) {
		t.Fatalf("%s %v", d.Body, err)
	}
}

func TestTagsAreCleanedAndSaved(t *testing.T) {
	s := testStore(t)
	d, err := s.CreateDocument(101)
	if err != nil || len(d.Tags) != 0 {
		t.Fatalf("%+v %v", d, err)
	}
	tags := []string{"  Draft ", "draft", "Needs   review", ""}
	d, err = s.UpdateDocument(101, d.ID, DocumentPatch{Tags: &tags, Revision: d.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(d.Tags, "|") != "Draft|Needs review" {
		t.Fatalf("tags %q", d.Tags)
	}
	long := []string{strings.Repeat("x", 41)}
	if _, err := s.UpdateDocument(101, d.ID, DocumentPatch{Tags: &long, Revision: d.Revision}); err == nil {
		t.Fatal("accepted an over-long tag")
	}
	// Editing the title leaves tags alone.
	title := "Spec"
	d, err = s.UpdateDocument(101, d.ID, DocumentPatch{Title: &title, Revision: d.Revision})
	if err != nil || len(d.Tags) != 2 {
		t.Fatalf("%+v %v", d, err)
	}
}
