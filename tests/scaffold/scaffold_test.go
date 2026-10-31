// Package scaffold checks that the repository layout matches the architecture's
// service set (AD-1), database ownership (AD-2) and network exposure (AD-14).
package scaffold

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var services = []string{
	"api-gateway",
	"identity-service",
	"catalog-service",
	"order-service",
	"payment-service",
	"shipping-simulator",
	"analytics-service",
}

// dataServices own a database; api-gateway owns no data.
var dataServices = services[1:]

// grpcServices expose a gRPC contract under proto/<name>/v1.
var grpcServices = []string{"identity", "catalog", "order", "payment", "analytics"}

// publishedPorts are the only compose services allowed to publish host ports.
var publishedPorts = []string{"api-gateway", "jaeger", "prometheus", "grafana", "kafka-ui"}

var infra = []string{"postgres", "kafka", "jaeger", "prometheus", "grafana", "kafka-ui"}

const root = "../.."

func read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func sorted(s []string) []string {
	c := slices.Clone(s)
	slices.Sort(c)
	return c
}

func assertSet(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(sorted(got), sorted(want)) {
		t.Errorf("%s = %v, want exactly %v", what, sorted(got), sorted(want))
	}
}

func isDir(rel string) bool {
	fi, err := os.Stat(filepath.Join(root, rel))
	return err == nil && fi.IsDir()
}

func TestGoWorkListsExactlyTheServices(t *testing.T) {
	re := regexp.MustCompile(`\./services/([a-z-]+)`)
	var got []string
	for _, m := range re.FindAllStringSubmatch(read(t, "go.work"), -1) {
		got = append(got, m[1])
	}
	assertSet(t, "go.work services", got, services)
}

func TestMakefileServicesList(t *testing.T) {
	m := regexp.MustCompile(`(?m)^SERVICES\s*:?=\s*(.+)$`).FindStringSubmatch(read(t, "Makefile"))
	if m == nil {
		t.Fatal("Makefile has no SERVICES variable")
	}
	assertSet(t, "Makefile SERVICES", strings.Fields(m[1]), services)
}

func TestCIFiltersListExactlyTheServices(t *testing.T) {
	var ci struct {
		Jobs map[string]struct {
			Steps []struct {
				ID   string `yaml:"id"`
				With map[string]string
			}
		}
	}
	if err := yaml.Unmarshal([]byte(read(t, ".github/workflows/ci.yml")), &ci); err != nil {
		t.Fatal(err)
	}
	var filters string
	for _, j := range ci.Jobs {
		for _, s := range j.Steps {
			if s.ID == "filter" {
				filters = s.With["filters"]
			}
		}
	}
	if filters == "" {
		t.Fatal("ci.yml has no paths-filter step with id 'filter'")
	}
	var f map[string]any
	if err := yaml.Unmarshal([]byte(filters), &f); err != nil {
		t.Fatal(err)
	}
	var got []string
	for k := range f {
		got = append(got, k)
	}
	assertSet(t, "ci.yml filters", got, services)
}

type composeFile struct {
	Services map[string]struct {
		Ports       []any             `yaml:"ports"`
		Networks    []string          `yaml:"networks"`
		Environment map[string]string `yaml:"environment"`
		Healthcheck struct {
			Test []string `yaml:"test"`
		} `yaml:"healthcheck"`
	}
	Networks map[string]any
}

func loadCompose(t *testing.T) composeFile {
	t.Helper()
	var c composeFile
	if err := yaml.Unmarshal([]byte(read(t, "deployments/docker-compose.yml")), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestComposeRunsServicesAndInfraOnOneNetwork(t *testing.T) {
	c := loadCompose(t)
	var got []string
	for name := range c.Services {
		got = append(got, name)
	}
	assertSet(t, "compose services", got, append(slices.Clone(services), infra...))
	if len(c.Networks) != 1 {
		t.Errorf("compose networks = %d, want exactly 1", len(c.Networks))
	}
	for name, s := range c.Services {
		if len(s.Networks) > 1 {
			t.Errorf("%s joins %d networks, want at most 1", name, len(s.Networks))
		}
	}
}

func TestComposePublishesOnlyGatewayAndObservabilityPorts(t *testing.T) {
	var got []string
	for name, s := range loadCompose(t).Services {
		if len(s.Ports) > 0 {
			got = append(got, name)
		}
	}
	assertSet(t, "services publishing host ports", got, publishedPorts)
}

func TestDataServicesHaveHexagonalLayout(t *testing.T) {
	for _, s := range dataServices {
		for _, d := range []string{"cmd/server", "internal/domain", "internal/service", "internal/repository", "internal/transport", "migrations"} {
			if !isDir(filepath.Join("services", s, d)) {
				t.Errorf("services/%s/%s missing", s, d)
			}
		}
	}
}

func TestRetiredServicesAreGone(t *testing.T) {
	for _, s := range []string{"customer-service", "review-service"} {
		if isDir(filepath.Join("services", s)) {
			t.Errorf("services/%s still exists", s)
		}
	}
}

func TestSharedLayoutExists(t *testing.T) {
	dirs := []string{"contracts/events/v1", "pkg/outbox", "pkg/inbox", "proto/gen/go"}
	for _, g := range grpcServices {
		dirs = append(dirs, filepath.Join("proto", g, "v1"))
	}
	for _, d := range dirs {
		if !isDir(d) {
			t.Errorf("%s missing", d)
		}
	}
}

func TestPostgresScriptsListOneDatabasePerDataService(t *testing.T) {
	var want []string
	for _, s := range dataServices {
		want = append(want, dbName(s))
	}
	re := regexp.MustCompile(`(?m)^DATABASES="([^"]*)"`)
	for _, script := range []string{"deployments/postgres/init/01-databases.sh", "deployments/postgres/check-isolation.sh"} {
		m := re.FindStringSubmatch(read(t, script))
		if m == nil {
			t.Errorf("%s has no DATABASES=\"...\" line", script)
			continue
		}
		assertSet(t, script+" DATABASES", strings.Fields(m[1]), want)
	}
}

// composeDefaults matches ${VAR:-default}; replacing it with $1 yields the default.
var composeDefaults = regexp.MustCompile(`\$\{[A-Z_]+:-([^}]*)\}`)

func TestDataServicesConnectToTheirOwnDatabase(t *testing.T) {
	c := loadCompose(t)
	for _, s := range dataServices {
		raw := composeDefaults.ReplaceAllString(c.Services[s].Environment["DATABASE_URL"], "$1")
		u, err := url.Parse(raw)
		if err != nil || raw == "" {
			t.Errorf("%s DATABASE_URL %q: %v", s, raw, err)
			continue
		}
		want := dbName(s)
		if u.User.Username() != want || strings.TrimPrefix(u.Path, "/") != want {
			t.Errorf("%s connects as %q to database %q, want %q to %q", s, u.User.Username(), strings.TrimPrefix(u.Path, "/"), want, want)
		}
	}
}

// dbName is the database (and DB user) name of a service: identity-service → identity.
func dbName(service string) string {
	return strings.TrimSuffix(strings.TrimSuffix(service, "-service"), "-simulator")
}

func TestEveryServiceHasAReadinessHealthcheck(t *testing.T) {
	c := loadCompose(t)
	want := []string{"CMD", "/server", "healthcheck"}
	for _, s := range services {
		if got := c.Services[s].Healthcheck.Test; !slices.Equal(got, want) {
			t.Errorf("%s healthcheck = %v, want %v", s, got, want)
		}
	}
}

func TestPrometheusScrapesEveryServiceOpsPort(t *testing.T) {
	var cfg struct {
		ScrapeConfigs []struct {
			StaticConfigs []struct {
				Targets []string `yaml:"targets"`
			} `yaml:"static_configs"`
		} `yaml:"scrape_configs"`
	}
	if err := yaml.Unmarshal([]byte(read(t, "deployments/prometheus/prometheus.yml")), &cfg); err != nil {
		t.Fatal(err)
	}
	var targets []string
	for _, sc := range cfg.ScrapeConfigs {
		for _, st := range sc.StaticConfigs {
			targets = append(targets, st.Targets...)
		}
	}
	for _, s := range services {
		if !slices.Contains(targets, s+":8090") {
			t.Errorf("prometheus does not scrape %s:8090 (targets %v)", s, targets)
		}
	}
}

func TestGrafanaProvisionsPrometheusAndJaeger(t *testing.T) {
	var cfg struct {
		Datasources []struct {
			Type string `yaml:"type"`
			URL  string `yaml:"url"`
		} `yaml:"datasources"`
	}
	if err := yaml.Unmarshal([]byte(read(t, "deployments/grafana/datasources.yml")), &cfg); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, d := range cfg.Datasources {
		got[d.Type] = d.URL
	}
	if got["prometheus"] != "http://prometheus:9090" || got["jaeger"] != "http://jaeger:16686" {
		t.Errorf("grafana data sources = %v, want prometheus and jaeger", got)
	}
}

func TestEveryServiceExportsTracesToJaeger(t *testing.T) {
	c := loadCompose(t)
	for _, s := range services {
		if got := c.Services[s].Environment["OTEL_EXPORTER_OTLP_ENDPOINT"]; got != "http://jaeger:4318" {
			t.Errorf("%s OTEL_EXPORTER_OTLP_ENDPOINT = %q, want http://jaeger:4318", s, got)
		}
	}
}
