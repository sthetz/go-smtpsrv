package smtpsrv

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
)

const (
	contentTypeMultipartMixed       = "multipart/mixed"
	contentTypeMultipartAlternative = "multipart/alternative"
	contentTypeMultipartRelated     = "multipart/related"
	contentTypeTextHtml             = "text/html"
	contentTypeTextPlain            = "text/plain"
)

// ParseEmail Parse an email message read from io.Reader into parsemail.Email struct
func ParseEmail(r io.Reader) (email *Email, err error) {
	msg, err := mail.ReadMessage(r)
	if err != nil {
		return
	}

	email, err = createEmailFromHeader(msg.Header)
	if err != nil {
		return
	}

	email.ContentType = msg.Header.Get("Content-Type")
	contentType, params, err := parseContentType(email.ContentType)
	if err != nil {
		return
	}

	switch contentType {
	case contentTypeMultipartMixed:
		email.TextBody, email.HTMLBody, email.Attachments, email.EmbeddedFiles, err = parseMultipartMixed(msg.Body, params["boundary"])
	case contentTypeMultipartAlternative:
		email.TextBody, email.HTMLBody, email.EmbeddedFiles, err = parseMultipartAlternative(msg.Body, params["boundary"])
	case contentTypeMultipartRelated:
		email.TextBody, email.HTMLBody, email.EmbeddedFiles, err = parseMultipartRelated(msg.Body, params["boundary"])
	case contentTypeTextPlain:
		newPart, err := decodeContent(msg.Body, msg.Header.Get("Content-Transfer-Encoding"), msg.Header.Get("Content-Type"))
		if err != nil {
			return email, err
		}

		message, _ := io.ReadAll(newPart)
		email.TextBody = strings.TrimSuffix(string(message[:]), "\n")
	case contentTypeTextHtml:
		newPart, err := decodeContent(msg.Body, msg.Header.Get("Content-Transfer-Encoding"), msg.Header.Get("Content-Type"))
		if err != nil {
			return email, err
		}

		message, err := io.ReadAll(newPart)
		if err != nil {
			return email, err
		}

		email.HTMLBody = strings.TrimSuffix(string(message[:]), "\n")
	default:
		email.Content, err = decodeContent(msg.Body, msg.Header.Get("Content-Transfer-Encoding"), msg.Header.Get("Content-Type"))
	}

	return
}

func createEmailFromHeader(header mail.Header) (email *Email, err error) {
	hp := headerParser{header: &header}

	email = &Email{}
	email.Subject = decodeMimeSentence(header.Get("Subject"))
	email.From = hp.parseAddressList(header.Get("From"))
	email.Sender = hp.parseAddress(header.Get("Sender"))
	email.ReplyTo = hp.parseAddressList(header.Get("Reply-To"))
	email.To = hp.parseAddressList(header.Get("To"))
	email.Cc = hp.parseAddressList(header.Get("Cc"))
	email.Bcc = hp.parseAddressList(header.Get("Bcc"))
	email.Date = hp.parseTime(header.Get("Date"))
	email.ResentFrom = hp.parseAddressList(header.Get("Resent-From"))
	email.ResentSender = hp.parseAddress(header.Get("Resent-Sender"))
	email.ResentTo = hp.parseAddressList(header.Get("Resent-To"))
	email.ResentCc = hp.parseAddressList(header.Get("Resent-Cc"))
	email.ResentBcc = hp.parseAddressList(header.Get("Resent-Bcc"))
	email.ResentMessageID = hp.parseMessageId(header.Get("Resent-Message-ID"))
	email.MessageID = hp.parseMessageId(header.Get("Message-ID"))
	email.InReplyTo = hp.parseMessageIdList(header.Get("In-Reply-To"))
	email.References = hp.parseMessageIdList(header.Get("References"))
	email.ResentDate = hp.parseTime(header.Get("Resent-Date"))

	if hp.err != nil {
		err = hp.err
		return
	}

	// decode whole header for easier access to extra fields
	// todo: should we decode? aren't only standard fields mime encoded?
	email.Header, err = decodeHeaderMime(header)
	if err != nil {
		return
	}

	return
}

func parseContentType(contentTypeHeader string) (contentType string, params map[string]string, err error) {
	if contentTypeHeader == "" {
		contentType = contentTypeTextPlain
		return
	}

	mediaType, params, err := mime.ParseMediaType(sanitizeContentTypeHeader(contentTypeHeader))
	if err != nil {
		err = fmt.Errorf("error parsing media type from content type header %s: %s", contentTypeHeader, err)
	}

	return mediaType, params, err
}

func parseMultipartRelated(msg io.Reader, boundary string) (textBody, htmlBody string, embeddedFiles []EmbeddedFile, err error) {
	pmr := multipart.NewReader(msg, boundary)
	for {
		part, err := pmr.NextPart()

		if err == io.EOF {
			break
		} else if err != nil {
			return textBody, htmlBody, embeddedFiles, err
		}

		contentType, params, err := mime.ParseMediaType(sanitizeContentTypeHeader(part.Header.Get("Content-Type")))
		if err != nil {
			err = fmt.Errorf("error parsing media type from content type header %s: %s", part.Header.Get("Content-Type"), err)
			return textBody, htmlBody, embeddedFiles, err
		}

		switch contentType {
		case contentTypeTextPlain:
			ppContent, err := io.ReadAll(part)
			if err != nil {
				return textBody, htmlBody, embeddedFiles, err
			}

			textBody += strings.TrimSuffix(string(ppContent[:]), "\n")
		case contentTypeTextHtml:
			ppContent, err := io.ReadAll(part)
			if err != nil {
				return textBody, htmlBody, embeddedFiles, err
			}

			htmlBody += strings.TrimSuffix(string(ppContent[:]), "\n")
		case contentTypeMultipartAlternative:
			tb, hb, ef, err := parseMultipartAlternative(part, params["boundary"])
			if err != nil {
				return textBody, htmlBody, embeddedFiles, err
			}

			htmlBody += hb
			textBody += tb
			embeddedFiles = append(embeddedFiles, ef...)
		default:
			if isEmbeddedFile(part) {
				ef, err := decodeEmbeddedFile(part)
				if err != nil {
					return textBody, htmlBody, embeddedFiles, err
				}

				embeddedFiles = append(embeddedFiles, ef)
			} else {
				return textBody, htmlBody, embeddedFiles, fmt.Errorf("can't process multipart/related inner mime type: %s", contentType)
			}
		}
	}

	return textBody, htmlBody, embeddedFiles, err
}

func decodeCharset(content io.Reader, contentTypeWithCharset string) io.Reader {
	charset := "default"
	if strings.Contains(contentTypeWithCharset, "; charset=") {
		split := strings.Split(contentTypeWithCharset, "; charset=")
		charset = strings.ToLower(strings.Trim(split[1], " \"'\n\r"))
	}

	decoders := map[string]*charmap.Charmap{
		"windows-1252": charmap.Windows1252,
		"iso-8859-1":   charmap.ISO8859_1,
		"koi8-r":       charmap.KOI8R,
		"windows-1251": charmap.Windows1251,
	}

	if charset != "default" {
		if decoder, ok := decoders[charset]; ok {
			return decoder.NewDecoder().Reader(content)
		}
	}

	return content
}

func parseMultipartAlternative(msg io.Reader, boundary string) (textBody, htmlBody string, embeddedFiles []EmbeddedFile, err error) {
	pmr := multipart.NewReader(msg, boundary)
	for {
		part, err := pmr.NextPart()

		if err == io.EOF {
			break
		} else if err != nil {
			return textBody, htmlBody, embeddedFiles, err
		}

		contentType, params, err := mime.ParseMediaType(sanitizeContentTypeHeader(part.Header.Get("Content-Type")))
		if err != nil {
			err = fmt.Errorf("error parsing media type from content type header %s: %s", part.Header.Get("Content-Type"), err)
			return textBody, htmlBody, embeddedFiles, err
		}

		switch contentType {
		case contentTypeTextPlain:
			newPart, err := decodeContent(part, part.Header.Get("Content-Transfer-Encoding"), part.Header.Get("Content-Type"))
			if err != nil {
				return textBody, htmlBody, embeddedFiles, err
			}

			ppContent, err := io.ReadAll(newPart)
			if err != nil {
				return textBody, htmlBody, embeddedFiles, err
			}

			textBody += strings.TrimSuffix(string(ppContent[:]), "\n")

		case contentTypeTextHtml:
			newPart, err := decodeContent(part, part.Header.Get("Content-Transfer-Encoding"), part.Header.Get("Content-Type"))
			if err != nil {
				return textBody, htmlBody, embeddedFiles, err
			}

			ppContent, err := io.ReadAll(newPart)
			if err != nil {
				return textBody, htmlBody, embeddedFiles, err
			}

			htmlBody += strings.TrimSuffix(string(ppContent[:]), "\n")

		case contentTypeMultipartRelated:
			tb, hb, ef, err := parseMultipartRelated(part, params["boundary"])
			if err != nil {
				return textBody, htmlBody, embeddedFiles, err
			}

			htmlBody += hb
			textBody += tb
			embeddedFiles = append(embeddedFiles, ef...)

		default:
			if isEmbeddedFile(part) {
				ef, err := decodeEmbeddedFile(part)
				if err != nil {
					return textBody, htmlBody, embeddedFiles, err
				}

				embeddedFiles = append(embeddedFiles, ef)
			} else {
				return textBody, htmlBody, embeddedFiles, fmt.Errorf("can't process multipart/alternative inner mime type: %s", contentType)
			}
		}
	}

	return textBody, htmlBody, embeddedFiles, err
}

func parseMultipartMixed(msg io.Reader, boundary string) (textBody, htmlBody string, attachments []Attachment, embeddedFiles []EmbeddedFile, err error) {
	mr := multipart.NewReader(msg, boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			return textBody, htmlBody, attachments, embeddedFiles, err
		}

		contentType, params, err := mime.ParseMediaType(sanitizeContentTypeHeader(part.Header.Get("Content-Type")))
		if err != nil {
			err = fmt.Errorf("error parsing media type from content type header %s: %s", part.Header.Get("Content-Type"), err)
			return textBody, htmlBody, attachments, embeddedFiles, err
		}

		if contentType == contentTypeMultipartAlternative {
			textBody, htmlBody, embeddedFiles, err = parseMultipartAlternative(part, params["boundary"])
			if err != nil {
				return textBody, htmlBody, attachments, embeddedFiles, err
			}
		} else if contentType == contentTypeMultipartRelated {
			textBody, htmlBody, embeddedFiles, err = parseMultipartRelated(part, params["boundary"])
			if err != nil {
				return textBody, htmlBody, attachments, embeddedFiles, err
			}
		} else if contentType == contentTypeTextPlain {
			newPart, err := decodeContent(part, part.Header.Get("Content-Transfer-Encoding"), part.Header.Get("Content-Type"))
			if err != nil {
				return textBody, htmlBody, attachments, embeddedFiles, err
			}

			ppContent, err := io.ReadAll(newPart)
			if err != nil {
				return textBody, htmlBody, attachments, embeddedFiles, err
			}

			textBody += strings.TrimSuffix(string(ppContent[:]), "\n")
		} else if contentType == contentTypeTextHtml {
			newPart, err := decodeContent(part, part.Header.Get("Content-Transfer-Encoding"), part.Header.Get("Content-Type"))
			if err != nil {
				return textBody, htmlBody, attachments, embeddedFiles, err
			}

			ppContent, err := io.ReadAll(newPart)
			if err != nil {
				return textBody, htmlBody, attachments, embeddedFiles, err
			}

			htmlBody += strings.TrimSuffix(string(ppContent[:]), "\n")
		} else if isAttachment(part, contentType) {
			at, err := decodeAttachment(part)
			if err != nil {
				return textBody, htmlBody, attachments, embeddedFiles, err
			}

			attachments = append(attachments, at)
		} else {
			return textBody, htmlBody, attachments, embeddedFiles, fmt.Errorf("unknown multipart/mixed nested mime type: %s", contentType)
		}
	}

	return textBody, htmlBody, attachments, embeddedFiles, err
}

func decodeMimeSentence(s string) string {
	var result []string

	success, str := decodeKoi8(s)
	if success {
		return str
	}

	ss := strings.Split(s, " ")

	for _, word := range ss {
		dec := new(mime.WordDecoder)
		w, err := dec.Decode(word)
		if err != nil {
			if len(result) == 0 {
				w = word
			} else {
				w = " " + word
			}
		}

		result = append(result, w)
	}

	return strings.Join(result, "")
}

func decodeKoi8(s string) (bool, string) {   
	if !(strings.HasPrefix(strings.ToLower(s), "=?koi8-r")) {
		return false, ""
	}
	
	const prefixLen = 11 // =?KOI8-R?B? or =?KOI8-R?Q?
	prefix := strings.ToLower(s[0:prefixLen])
	origin := s[prefixLen:(len(s) - 2)]
   
	var decodedOrigin []byte
	var err error
   
	if prefix[prefixLen-2] == 'b' {
	 	decodedOrigin, err = base64.StdEncoding.DecodeString(origin)
	 	if err != nil {
	  		fmt.Println("Decode failed (base64):", origin)
			return false, ""
		}
	} else if prefix[prefixLen-2] == 'q' {
	 	reader := quotedprintable.NewReader(strings.NewReader(origin))
	 	decodedOrigin, err = io.ReadAll(reader)
	 	if err != nil {
	  		fmt.Println("Decode failed (quoted-printable):", origin)
			return false, ""
		}
	} else {
		fmt.Println("Unknown encoding", origin)
		return false, ""
	}
   
	decoder := charmap.KOI8R.NewDecoder()
	reader := decoder.Reader(strings.NewReader(string(decodedOrigin)))
	decodedString, err := io.ReadAll(reader)
	if err != nil {
		fmt.Println("Decode failed (koi8):", decodedOrigin)
		return false, ""
	}
   
	return true, string(decodedString)
}

func decodeHeaderMime(header mail.Header) (mail.Header, error) {
	parsedHeader := map[string][]string{}

	for headerName, headerData := range header {

		var parsedHeaderData []string
		for _, headerValue := range headerData {
			parsedHeaderData = append(parsedHeaderData, decodeMimeSentence(headerValue))
		}

		parsedHeader[headerName] = parsedHeaderData
	}

	return parsedHeader, nil
}

func isEmbeddedFile(part *multipart.Part) bool {
	return part.Header.Get("Content-Transfer-Encoding") != ""
}

func decodeEmbeddedFile(part *multipart.Part) (ef EmbeddedFile, err error) {
	cid := decodeMimeSentence(part.Header.Get("Content-Id"))
	decoded, err := decodeContent(part, part.Header.Get("Content-Transfer-Encoding"), part.Header.Get("Content-Type"))
	if err != nil {
		return
	}

	ef.CID = strings.Trim(cid, "<>")
	ef.Data = decoded
	ef.ContentType = part.Header.Get("Content-Type")

	return
}

func isAttachment(part *multipart.Part, contentType string) bool {
	return part.FileName() != "" || contentType == "application/octet-stream"
}

func decodeAttachment(part *multipart.Part) (at Attachment, err error) {
	filename := decodeMimeSentence(part.FileName())
	if filename == "" {
		filename = fmt.Sprintf("attachment-%d", time.Now().UnixNano())
	}
	decoded, err := decodeContent(part, part.Header.Get("Content-Transfer-Encoding"), part.Header.Get("Content-Type"))
	if err != nil {
		return
	}

	at.Filename = filename
	at.Data = decoded
	at.ContentType = strings.Split(part.Header.Get("Content-Type"), ";")[0]

	if at.ContentType == "application/octet-stream" {
		data, err := io.ReadAll(part)
		if err != nil {
			return at, err
		}

		at.Data = bytes.NewReader(data)
	}

	return
}

func decodeContent(content io.Reader, encoding string, contentTypeWithCharset string) (io.Reader, error) {
	switch strings.ToLower(encoding) {
	case "base64":
		decoded := base64.NewDecoder(base64.StdEncoding, content)
		b, err := io.ReadAll(decoded)
		if err != nil {
			return nil, err
		}

		return decodeCharset(bytes.NewReader(b), contentTypeWithCharset), nil

	case "7bit", "8bit":
		dd, err := io.ReadAll(content)
		if err != nil {
			return nil, err
		}

		return decodeCharset(bytes.NewReader(dd), contentTypeWithCharset), nil

	case "quoted-printable":
		decoded := quotedprintable.NewReader(content)
		b, err := io.ReadAll(decoded)
		if err != nil {
			return nil, err
		}

		return decodeCharset(bytes.NewReader(b), contentTypeWithCharset), nil
	case "":
		return decodeCharset(content, contentTypeWithCharset), nil

	default:
		return nil, fmt.Errorf("unknown encoding: %s", encoding)
	}
}

type headerParser struct {
	header *mail.Header
	err    error
}

func (hp headerParser) parseAddress(s string) (ma *mail.Address) {
	if hp.err != nil {
		return nil
	}

	if strings.Trim(s, " \n") != "" {
		ma, hp.err = mail.ParseAddress(s)

		return ma
	}

	return nil
}

func (hp headerParser) parseAddressList(s string) (ma []*mail.Address) {
	if hp.err != nil {
		return
	}

	if strings.Trim(s, " \n") != "" {
		ma, hp.err = mail.ParseAddressList(s)
		return
	}

	return
}

func (hp headerParser) parseTime(s string) (t time.Time) {
	if hp.err != nil || s == "" {
		return
	}

	formats := []string{
		time.RFC1123Z,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		time.RFC1123Z + " (MST)",
		"Mon, 2 Jan 2006 15:04:05 -0700 (MST)",
	}

	for _, format := range formats {
		t, hp.err = time.Parse(format, s)
		if hp.err == nil {
			return
		}
	}

	return
}

func (hp headerParser) parseMessageId(s string) string {
	if hp.err != nil {
		return ""
	}

	return strings.Trim(s, "<> ")
}

func (hp headerParser) parseMessageIdList(s string) (result []string) {
	if hp.err != nil {
		return
	}

	for _, p := range strings.Split(s, " ") {
		if strings.Trim(p, " \n") != "" {
			result = append(result, hp.parseMessageId(p))
		}
	}

	return
}

// sanitizeContentTypeHeader removes duplicate parameters from the content type header if they are equal in lowercase
// and with or without quotes e.g. text/html; charset=utf-8; charset="UTF-8" -> text/html; charset=utf-8
func sanitizeContentTypeHeader(contentType string) string {
	params := strings.Split(contentType, ";")
	seen := make(map[string]struct{}, len(params))
	result := make([]string, 0, len(params))
	for _, p := range params {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		if strings.Contains(p, "=") {
			split := strings.Split(p, "=")
			if _, ok := seen[strings.ToLower(split[0])]; ok {
				continue
			}

			seen[strings.ToLower(split[0])] = struct{}{}
		}

		result = append(result, p)
	}

	return strings.Join(result, "; ")
}

// Attachment with filename, content type and data (as an io.Reader)
type Attachment struct {
	Filename    string
	ContentType string
	Data        io.Reader
}

// EmbeddedFile with content id, content type and data (as an io.Reader)
type EmbeddedFile struct {
	CID         string
	ContentType string
	Data        io.Reader
}

// Email with fields for all the headers defined in RFC5322 with its attachments and embedded files
type Email struct {
	Header mail.Header

	Subject    string
	Sender     *mail.Address
	From       []*mail.Address
	ReplyTo    []*mail.Address
	To         []*mail.Address
	Cc         []*mail.Address
	Bcc        []*mail.Address
	Date       time.Time
	MessageID  string
	InReplyTo  []string
	References []string

	ResentFrom      []*mail.Address
	ResentSender    *mail.Address
	ResentTo        []*mail.Address
	ResentDate      time.Time
	ResentCc        []*mail.Address
	ResentBcc       []*mail.Address
	ResentMessageID string

	ContentType string
	Content     io.Reader

	HTMLBody string
	TextBody string

	Attachments   []Attachment
	EmbeddedFiles []EmbeddedFile
}
