// Command continuum-plugin-ebooksdb is the plugin entrypoint.
package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/hashicorp/go-hclog"

	publicmanifest "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/manifest"
	sdkruntime "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtime"

	"github.com/ContinuumApp/continuum-plugin-ebooksdb/internal/httproutes"
	pluginrt "github.com/ContinuumApp/continuum-plugin-ebooksdb/internal/runtime"
)

//go:embed manifest.json
var manifestRaw []byte

func main() {
	logger := hclog.New(&hclog.LoggerOptions{Name: "continuum-plugin-ebooksdb"})

	manifest, err := publicmanifest.Load(manifestRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load manifest: %v\n", err)
		os.Exit(1)
	}

	httpSrv := httproutes.NewServer()

	rt := pluginrt.New(manifest, func(cfg pluginrt.Config) error {
		return nil
	})

	sdkruntime.Serve(sdkruntime.ServeConfig{
		Logger: logger,
		Servers: sdkruntime.CapabilityServers{
			Runtime:    rt,
			HttpRoutes: httpSrv,
		},
	})
}
