package smtpsrv

import "testing"

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
