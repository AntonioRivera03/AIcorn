package knowledge

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// EmailDetails are an imported email's headers.
type EmailDetails struct {
	From string `json:"from"`
	To   string `json:"to,omitempty"`
	Cc   string `json:"cc,omitempty"`
	// Date is RFC 3339 when the header parses, otherwise as sent.
	Date string `json:"date,omitempty"`
}

type parsedEmail struct {
	Subject string
	Details EmailDetails
	Text    string
}

var errNotEmail = errors.New("not an email")

// parseEmail reads an RFC 5322 message (a .eml file): its headers, and its
// body as plain text, preferring a text/plain part over HTML. Attachments are
// skipped; the original file is kept with the document.
func parseEmail(content []byte) (parsedEmail, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(content))
	if err != nil {
		return parsedEmail{}, errNotEmail
	}
	if msg.Header.Get("From") == "" && msg.Header.Get("Subject") == "" {
		return parsedEmail{}, errNotEmail
	}
	decoder := &mime.WordDecoder{CharsetReader: charsetReader}
	header := func(name string) string {
		raw := msg.Header.Get(name)
		if decoded, err := decoder.DecodeHeader(raw); err == nil {
			return strings.TrimSpace(decoded)
		}
		return strings.TrimSpace(raw)
	}
	addresses := func(name string) string {
		parser := mail.AddressParser{WordDecoder: decoder}
		list, err := parser.ParseList(msg.Header.Get(name))
		if err != nil {
			return header(name)
		}
		out := make([]string, 0, len(list))
		for _, a := range list {
			if a.Name != "" {
				out = append(out, fmt.Sprintf("%s <%s>", a.Name, a.Address))
			} else {
				out = append(out, a.Address)
			}
		}
		return strings.Join(out, ", ")
	}

	e := parsedEmail{
		Subject: header("Subject"),
		Details: EmailDetails{From: addresses("From"), To: addresses("To"), Cc: addresses("Cc")},
	}
	if date, err := msg.Header.Date(); err == nil {
		e.Details.Date = date.UTC().Format(time.RFC3339)
	} else {
		e.Details.Date = header("Date")
	}
	plain, rich := emailText(textproto.MIMEHeader(msg.Header), msg.Body, 0)
	if plain == "" && rich != "" {
		plain = htmlToText(rich)
	}
	e.Text = tidyText(plain)
	return e, nil
}

// emailText walks a MIME entity and returns the first text/plain and
// text/html bodies it finds, decoded to UTF-8.
func emailText(header textproto.MIMEHeader, body io.Reader, depth int) (plain, rich string) {
	if depth > 10 {
		return "", ""
	}
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		mediaType, params = "text/plain", map[string]string{}
	}
	if disposition, _, _ := mime.ParseMediaType(header.Get("Content-Disposition")); disposition == "attachment" {
		return "", ""
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(body, params["boundary"])
		for {
			part, err := reader.NextRawPart()
			if err != nil {
				break
			}
			p, r := emailText(part.Header, part, depth+1)
			if plain == "" {
				plain = p
			}
			if rich == "" {
				rich = r
			}
		}
		return plain, rich
	}
	if mediaType != "text/plain" && mediaType != "text/html" {
		return "", ""
	}
	var decoded io.Reader = body
	switch strings.ToLower(header.Get("Content-Transfer-Encoding")) {
	case "quoted-printable":
		decoded = quotedprintable.NewReader(body)
	case "base64":
		decoded = base64.NewDecoder(base64.StdEncoding, body)
	}
	converted, err := charsetReader(params["charset"], decoded)
	if err != nil {
		return "", ""
	}
	raw, err := io.ReadAll(io.LimitReader(converted, MaxFileBytes))
	if err != nil && len(raw) == 0 {
		return "", ""
	}
	text := strings.ToValidUTF8(string(raw), "�")
	if mediaType == "text/html" {
		return "", text
	}
	return text, ""
}

// charsetReader decodes the charsets common in email. Anything else passes
// through when it's already valid UTF-8.
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(charset)) {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return input, nil
	case "iso-8859-1", "latin1", "windows-1252", "cp1252":
		raw, err := io.ReadAll(io.LimitReader(input, MaxFileBytes))
		if err != nil {
			return nil, err
		}
		runes := make([]rune, len(raw))
		for i, b := range raw {
			runes[i] = rune(b)
		}
		return strings.NewReader(string(runes)), nil
	default:
		raw, err := io.ReadAll(io.LimitReader(input, MaxFileBytes))
		if err != nil {
			return nil, err
		}
		if !utf8.Valid(raw) {
			return nil, fmt.Errorf("unsupported charset %q", charset)
		}
		return bytes.NewReader(raw), nil
	}
}

var (
	htmlHidden   = regexp.MustCompile(`(?is)<(style|script|head)[^>]*>.*?</(style|script|head)>`)
	htmlBreak    = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|tr|li|h[1-6]|blockquote)>`)
	htmlTag      = regexp.MustCompile(`(?s)<[^>]*>`)
	blankLines   = regexp.MustCompile(`\n{3,}`)
	spaceAtLines = regexp.MustCompile(`[ \t]+\n`)
)

// htmlToText is a readable fallback for HTML-only email, not a renderer.
func htmlToText(s string) string {
	s = htmlHidden.ReplaceAllString(s, "")
	s = htmlBreak.ReplaceAllString(s, "\n")
	s = htmlTag.ReplaceAllString(s, "")
	return html.UnescapeString(s)
}

func tidyText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, " ", " ")
	s = spaceAtLines.ReplaceAllString(s, "\n")
	s = blankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
