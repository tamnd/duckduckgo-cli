package duckduckgo

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes duckduckgo as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/duckduckgo-cli/duckduckgo"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// duckduckgo:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone duckduckgo binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the duckduckgo driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "duckduckgo",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "duckduckgo",
			Short:  "DuckDuckGo instant answers CLI",
			Long: `DuckDuckGo instant answers CLI

duckduckgo fetches instant answers, definitions, and topic summaries from
the DuckDuckGo Instant Answer API. No API key required. Clean JSON output
that pipes into the rest of your tools.`,
			Site: Host,
			Repo: "https://github.com/tamnd/duckduckgo-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// Search op: instant answer for a query. Single=true because the API
	// returns one answer per query.
	kit.Handle(app, kit.OpMeta{
		Name:    "search",
		Group:   "read",
		Single:  true,
		Summary: "Get the instant answer for a topic or query",
		Args:    []kit.Arg{{Name: "query", Help: "search query", Variadic: true}},
	}, search)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type searchIn struct {
	Query        []string `kit:"arg,variadic" help:"search query"`
	SkipDisambig bool     `kit:"flag" help:"skip disambiguation pages" default:"true"`
	Client       *Client  `kit:"inject"`
}

// --- handlers ---

func search(ctx context.Context, in searchIn, emit func(*Answer) error) error {
	q := strings.Join(in.Query, " ")
	ans, err := in.Client.Search(ctx, q, in.SkipDisambig)
	if err != nil {
		return mapErr(err)
	}
	return emit(ans)
}

// --- Resolver: the URI-native string functions, pure and network-free ---

// Classify is a no-op for duckduckgo since the API uses free-text queries,
// not addressable resource IDs. We still satisfy the kit.Classifier interface
// by treating any input as an "answer" type keyed on the trimmed query string.
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty duckduckgo reference")
	}
	return "answer", input, nil
}

// Locate returns the DuckDuckGo search URL for a given (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	if uriType != "answer" {
		return "", errs.Usage("duckduckgo has no resource type %q", uriType)
	}
	return "https://duckduckgo.com/?q=" + id, nil
}

// --- helpers ---

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	return err
}
