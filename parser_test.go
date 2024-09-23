package smtpsrv

import (
	"bytes"
	"net/mail"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_sanitizeContentTypeHeader(t *testing.T) {
	type args struct {
		contentType string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Without duplicate params",
			args: args{
				contentType: "text/plain; charset=utf-8",
			},
			want: "text/plain; charset=utf-8",
		},
		{
			name: "With duplicate params in same case",
			args: args{
				contentType: "text/plain; charset=utf-8; charset=utf-8",
			},
			want: "text/plain; charset=utf-8",
		},
		{
			name: "With duplicate params in different case",
			args: args{
				contentType: "text/plain; charset=utf-8; charset=UTF-8",
			},
			want: "text/plain; charset=utf-8",
		},
		{
			name: "With duplicate params in different case and with quotes",
			args: args{
				contentType: "text/plain; charset=utf-8; charset=\"UTF-8\"",
			},
			want: "text/plain; charset=utf-8",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeContentTypeHeader(tt.args.contentType); got != tt.want {
				t.Errorf("sanitizeContentTypeHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseEmail(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *Email
	}{
		{
			name: "Email with UTF-8 charset",
			input: "From: sender@example.com\r\n" +
				"To: recipient@example.com\r\n" +
				"Subject: Test Email\r\n" +
				"Content-Type: text/plain; charset=utf-8\r\n" +
				"\r\n" +
				"Hello, this is a test email.",
			expected: &Email{
				From:        []*mail.Address{{Name: "", Address: "sender@example.com"}},
				To:          []*mail.Address{{Name: "", Address: "recipient@example.com"}},
				Subject:     "Test Email",
				ContentType: "text/plain; charset=utf-8",
				TextBody:    "Hello, this is a test email.",
			},
		},
		{
			name: "Email with KOI8-R charset and quoted-printable encoding",
			input: "From: sender@example.com\r\n" +
				"To: recipient@example.com\r\n" +
				"Subject: Test Email\r\n" +
				"Content-Type: text/plain; charset=koi8-r\r\n" +
				"Content-Transfer-Encoding: quoted-printable\r\n" +
				"\r\n" +
				"=EB=CF=CC=CC=C5=C7=C9, =C4=CF=C2=D2=D9=CA =C4=C5=CE=D8!=",
			expected: &Email{
				From:        []*mail.Address{{Name: "", Address: "sender@example.com"}},
				To:          []*mail.Address{{Name: "", Address: "recipient@example.com"}},
				Subject:     "Test Email",
				ContentType: "text/plain; charset=koi8-r",
				TextBody:    "Коллеги, добрый день!",
			},
		},
		{
			name: "Email with WINDOWS-1251 charset and Base64 encoding",
			input: "From: sender@example.com\r\n" +
				"To: recipient@example.com\r\n" +
				"Subject: Test Email\r\n" +
				"Content-Type: text/plain; charset=windows-1251\r\n" +
				"Content-Transfer-Encoding: Base64\r\n" +
				"\r\n" +
				"yu7r6+Xj6Cwg5O7h8PvpIOTl7fwh",
			expected: &Email{
				From:        []*mail.Address{{Name: "", Address: "sender@example.com"}},
				To:          []*mail.Address{{Name: "", Address: "recipient@example.com"}},
				Subject:     "Test Email",
				ContentType: "text/plain; charset=windows-1251",
				TextBody:    "Коллеги, добрый день!",
			},
		},
		{
			name: "Email with UTF-8 charset and quoted-printable encoding",
			input: "From: sender@example.com\r\n" +
				"To: recipient@example.com\r\n" +
				"Subject: Test\r\n" +
				"Content-Type: text/plain; charset=utf-8\r\n" +
				"Content-Transfer-Encoding: quoted-printable\r\n" +
				"\r\n" +
				"=D1=82=D0=B5=D1=81=D1=82=D0=BE =D1=82=D0=B5=D1=81=D1=82=",
			expected: &Email{
				From:        []*mail.Address{{Name: "", Address: "sender@example.com"}},
				To:          []*mail.Address{{Name: "", Address: "recipient@example.com"}},
				Subject:     "Test",
				ContentType: "text/plain; charset=utf-8",
				TextBody:    "тесто тест",
			},
		},
		{
			name: "Email with KOI8-R subject and base64 encoding",
			input: "From: sender@example.com\r\n" +
				"To: recipient@example.com\r\n" +
				"Subject: =?KOI8-R?B?79TFzNgg8s/Cyc7Tz84t08nUyQ==?=",
			expected: &Email{
				From:        []*mail.Address{{Name: "", Address: "sender@example.com"}},
				To:          []*mail.Address{{Name: "", Address: "recipient@example.com"}},
				Subject:     "Отель Робинсон-сити",
			},
		},
		{
			name: "Email with KOI8-R subject and quoted-printable encoding",
			input: "From: sender@example.com\r\n" +
				"To: recipient@example.com\r\n" +
				"Subject: =?koi8-r?Q?6=5F=F4=ED=5F15=2E05=2Erar?=",
			expected: &Email{
				From:        []*mail.Address{{Name: "", Address: "sender@example.com"}},
				To:          []*mail.Address{{Name: "", Address: "recipient@example.com"}},
				Subject:     "6_ТМ_15.05.rar",
			},
		},
		{
			name: "Email with WINDOWS-1251 subject and base64 encoding",
			input: "From: sender@example.com\r\n" +
				"To: recipient@example.com\r\n" +
				"Subject: =?Windows-1251?b?z+7k8uLl8Obk5e3o5SDk7vHy4OLq6CDx7u7h+eXt6P8g7eAg4OTw5fEgb3BAbnBwc2Vuc29yLnJ1?=",
			expected: &Email{
				From:        []*mail.Address{{Name: "", Address: "sender@example.com"}},
				To:          []*mail.Address{{Name: "", Address: "recipient@example.com"}},
				Subject:     "Подтверждение доставки сообщения на адрес op@nppsensor.ru",
			},
		},
		{
			name: "Email with ISO-8859 subject and base64 encoding",
			input: "From: sender@example.com\r\n" +
				"To: recipient@example.com\r\n" +
				"Subject: =?ISO-8859-1?q?Caf=E9?=",
			expected: &Email{
				From:        []*mail.Address{{Name: "", Address: "sender@example.com"}},
				To:          []*mail.Address{{Name: "", Address: "recipient@example.com"}},
				Subject:     "Café",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, err := ParseEmail(bytes.NewReader([]byte(tt.input)))

			require.NoError(t, err)
			require.Equal(t, tt.expected.TextBody, email.TextBody)
			require.Equal(t, tt.expected.Subject, email.Subject)
			require.Equal(t, tt.expected.From, email.From)
			require.Equal(t, tt.expected.To, email.To)
		})
	}
}
