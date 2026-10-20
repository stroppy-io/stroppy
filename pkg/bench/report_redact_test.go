package bench

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestReportDiagnosticRedaction(t *testing.T) {
	t.Parallel()

	password := "private-password"

	token := "private-token"
	for _, tc := range []struct {
		name          string
		configuration DriverConfig
		message       string
	}{
		{
			"postgres URI",
			DriverConfig{Kind: DriverPostgres, URL: "postgres://user:private-password@localhost/db"},
			"connection postgres://user:private-password@localhost/db failed: private-password",
		},
		{
			"postgres conninfo",
			DriverConfig{Kind: DriverPostgres, URL: "host=localhost password='private-password'"},
			"authentication failed: private-password",
		},
		{
			"mysql DSN",
			DriverConfig{Kind: DriverMySQL, URL: "user:private-password@tcp(localhost:3306)/db"},
			"connection user:private-password@tcp(localhost:3306)/db failed: private-password",
		},
		{
			"explicit auth",
			DriverConfig{Kind: DriverNoop, AuthToken: &token, AuthPassword: &password},
			"private-token private-password",
		},
		{
			"query credential",
			DriverConfig{Kind: DriverYDB, URL: "grpc://localhost/db?token=private-token"},
			"failed with private-token",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			redactor := reportRedactor(map[string]DriverConfig{"default": tc.configuration},
				&MetricsConfig{Headers: "authorization=private-header"})

			message := boundReportError(errors.New(tc.message+" private-header"), redactor)
			if strings.Contains(message, "private-") || !strings.Contains(message, "<redacted>") {
				t.Fatalf("credential remained: %s", message)
			}
		})
	}
}

func TestReportDecodedHeadersAndUsernames(t *testing.T) {
	t.Parallel()

	headers := "authorization=Bearer%20private-token"
	redactor := reportRedactor(map[string]DriverConfig{"default": {
		Kind: DriverPostgres, URL: "postgres://private-user:private-password@localhost/db",
	}}, &MetricsConfig{Headers: headers})

	message := boundReportError(errors.New("user=private-user Bearer private-token"), redactor)
	if strings.Contains(message, "private-") {
		t.Fatalf("decoded credentials retained: %s", message)
	}
}

func TestReportBearerTokenRedaction(t *testing.T) {
	t.Parallel()

	for _, headers := range []string{
		"authorization=Bearer%20private-token",
		"Authorization=bEaReR%20%20private-token",
		"x-api-key=private-token",
	} {
		redactor := reportRedactor(nil, &MetricsConfig{Headers: headers})
		for _, diagnostic := range []string{"private-token", "Bearer private-token"} {
			message := boundReportError(errors.New(diagnostic), redactor)
			if strings.Contains(message, "private-token") {
				t.Fatalf("header %q retained token: %s", headers, message)
			}
		}
	}
}

func TestReportInvalidConninfoDoesNotPublishSecrets(t *testing.T) {
	t.Parallel()

	connection := "host=localhost password=private-password auth_token=private-token sslmode=invalid"

	_, err := pgconn.ParseConfig(connection)
	if err == nil {
		t.Fatal("invalid connection accepted")
	}

	redactor := reportRedactor(map[string]DriverConfig{"default": {Kind: DriverPostgres, URL: connection}}, nil)

	message := boundReportError(err, redactor)
	if strings.Contains(message, "private-") || !strings.Contains(message, "cannot parse database connection string") {
		t.Fatalf("invalid conninfo exposed: %s", message)
	}

	diagnostic := (ReportContext{redactor: redactor}).Diagnostic(err.Error())
	if strings.Contains(diagnostic, "private-") {
		t.Fatalf("string diagnostic exposed invalid conninfo: %s", diagnostic)
	}
}

func TestReportRedactionPrecedesErrorTruncation(t *testing.T) {
	t.Parallel()

	password := strings.Repeat("private", 400)

	redactor := reportRedactor(map[string]DriverConfig{"default": {AuthPassword: &password}}, nil)
	if message := boundReportError(errors.New("failed: "+password), redactor); message != "failed: <redacted>" {
		t.Fatalf("partial credential retained: %s", message)
	}
}
