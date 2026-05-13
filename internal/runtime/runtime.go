// Package runtime implements the plugin's Runtime gRPC server. Config holds
// the parsed plugin global config; main.go uses the onConfigure callback to
// re-init pool/store/server when config arrives.
package runtime

import (
	"context"
	"errors"
	"sync"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/continuum/plugin/v1"
	"github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtimedefault"
)

// Config is the parsed plugin global config.
type Config struct {
	DatabaseURL string
}

// Server implements the plugin's Runtime service.
type Server struct {
	runtimedefault.Server
	manifest *pluginv1.PluginManifest
	onCfg    func(Config) error

	mu  sync.RWMutex
	cfg Config
}

// New constructs a runtime server. manifest may be nil in tests.
func New(manifest *pluginv1.PluginManifest, onConfigure func(Config) error) *Server {
	return &Server{manifest: manifest, onCfg: onConfigure}
}

func (s *Server) GetManifest(_ context.Context, _ *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

func (s *Server) Configure(_ context.Context, req *pluginv1.ConfigureRequest) (*pluginv1.ConfigureResponse, error) {
	cfg := Config{}
	for _, e := range req.GetConfig() {
		v := e.GetValue()
		if v == nil {
			continue
		}
		m := v.AsMap()
		val := m["value"]
		switch e.GetKey() {
		case "database_url":
			cfg.DatabaseURL = stringFrom(val)
		}
	}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("database_url is required")
	}
	if s.onCfg != nil {
		if err := s.onCfg(cfg); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return &pluginv1.ConfigureResponse{}, nil
}

// Snapshot returns the most recently applied Config.
func (s *Server) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func stringFrom(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// Compile-time check that Server satisfies the SDK interface.
var _ pluginv1.RuntimeServer = (*Server)(nil)
