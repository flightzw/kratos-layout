package apidoc

import (
	"fmt"
	"log"
	"net/http"
	"net/url"

	scalargo "github.com/bdpiprava/scalar-go"
)

type Server interface {
	HandleFunc(path string, h http.HandlerFunc)
}

type ScalarConfig struct {
	Port    string // Port to serve the scalar docs
	Docs    []byte // OpenAPI spec in bytes
	Path    string // Path to serve the scalar docs
	SpecURL string // Optional URL to serve the OpenAPI spec, if empty, Docs will be used
}

func WithLocalization(lang string) scalargo.Option {
	return func(o *scalargo.Options) {
		o.Configurations["localization"] = map[string]any{"locale": lang}
	}
}

func UseDefaultScalarOptions(title string) []scalargo.Option {
	return []scalargo.Option{
		scalargo.WithTheme(scalargo.ThemeKepler),
		scalargo.WithShowToolbar(scalargo.ShowToolbarLocalhost),
		scalargo.WithMetaDataOpts(scalargo.WithTitle(title)),
		WithLocalization("zh-CN"),
	}
}

func RegisterScalarDocHTTPServer(s Server, cfg ScalarConfig, opts ...scalargo.Option) {
	if cfg.SpecURL != "" {
		specURL, err := url.Parse(cfg.SpecURL)
		if err != nil {
			panic(fmt.Sprintf("invalid spec url: %s", cfg.SpecURL))
		}
		s.HandleFunc(specURL.Path, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, string(cfg.Docs))
		})
		log.Printf("registering openapi spec addr: http://localhost:%s%s", cfg.Port, specURL.Path)
		opts = append(opts, scalargo.WithSpecURL(cfg.SpecURL))
	} else {
		opts = append(opts, scalargo.WithSpecBytes(cfg.Docs))
	}
	s.HandleFunc(cfg.Path, func(w http.ResponseWriter, r *http.Request) {
		html, err := scalargo.NewV2(opts...)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		fmt.Fprint(w, html)
	})
	log.Printf("registering scalar api-reference addr: http://localhost:%s%s", cfg.Port, cfg.Path)
}
