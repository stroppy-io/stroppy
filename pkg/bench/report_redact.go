package bench

import (
	"net/url"
	"slices"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

// reportRedactor removes configured connection data from report diagnostics.
func reportRedactor(configurations map[string]DriverConfig, metrics *MetricsConfig) *strings.Replacer {
	values := []string{}
	for _, configuration := range configurations {
		values = append(values, driverReportSecrets(&configuration)...)
	}

	if metrics != nil {
		values = append(values, metrics.Headers)
		if headers, err := parseOTLPHeaders(metrics.Headers); err == nil {
			for _, value := range headers {
				values = append(values, value)
			}
		}

		for header := range strings.SplitSeq(metrics.Headers, ",") {
			if _, value, ok := strings.Cut(header, "="); ok {
				values = append(values, strings.TrimSpace(value))
			}
		}
	}

	patterns := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			patterns = append(patterns, value, url.QueryEscape(value), url.PathEscape(value))
		}
	}

	slices.SortFunc(patterns, func(a, b string) int { return len(b) - len(a) })

	const replacementPairSize = 2

	pairs := make([]string, 0, len(patterns)*replacementPairSize)
	for _, pattern := range patterns {
		pairs = append(pairs, pattern, "<redacted>")
	}

	return strings.NewReplacer(pairs...)
}

func driverReportSecrets(configuration *DriverConfig) []string {
	values := []string{configuration.URL}
	for _, value := range []*string{
		configuration.AuthToken, configuration.AuthUser, configuration.AuthPassword,
		configuration.ServiceAccountKeyFile,
	} {
		if value != nil {
			values = append(values, *value)
		}
	}

	values = append(values, uriReportSecrets(configuration.URL)...)

	switch configuration.Kind {
	case DriverPostgres, DriverPicodata:
		if parsed, err := pgconn.ParseConfig(configuration.URL); err == nil {
			values = append(values, parsed.User, parsed.Password)
			values = append(values, reportSecretValues(parsed.RuntimeParams)...)
		} else {
			values = append(values, err.Error())
		}
	case DriverMySQL:
		if parsed, err := mysql.ParseDSN(configuration.URL); err == nil {
			values = append(values, parsed.User, parsed.Passwd)
			values = append(values, reportSecretValues(parsed.Params)...)
		}
	default:
	}

	return values
}

func uriReportSecrets(connection string) []string {
	parsed, err := url.Parse(connection)
	if err != nil {
		return nil
	}

	values := []string{}

	if parsed.User != nil {
		password, _ := parsed.User.Password()
		values = append(values, parsed.User.Username(), password)
	}

	for key, items := range parsed.Query() {
		if secretReportKey(key) {
			values = append(values, items...)
		}
	}

	return values
}

func reportSecretValues(parameters map[string]string) []string {
	values := []string{}

	for key, value := range parameters {
		if secretReportKey(key) {
			values = append(values, value)
		}
	}

	return values
}

func secretReportKey(key string) bool {
	key = strings.ToLower(key)
	for _, marker := range []string{"password", "passwd", "token", "secret", "credential", "auth", "key"} {
		if strings.Contains(key, marker) {
			return true
		}
	}

	return false
}
