package cli

import (
	"encoding/json"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/catalog"
	"github.com/mik3dev/kie-cli/internal/config"
	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/spf13/cobra"
)

// newAPIClient resolves paths, config and key (auth error if missing) and builds a client.
func newAPIClient(cmd *cobra.Command, deps Deps) (*api.Client, config.Paths, error) {
	c, paths, _, err := newAPIClientCfg(cmd, deps)
	return c, paths, err
}

// newAPIClientCfg is newAPIClient that also returns the loaded config.
func newAPIClientCfg(cmd *cobra.Command, deps Deps) (*api.Client, config.Paths, config.Config, error) {
	paths, err := config.ResolvePaths(deps.Env)
	if err != nil {
		return nil, paths, config.Config{}, err
	}
	cfg, err := config.Load(paths, warnWriter(cmd, deps))
	if err != nil {
		return nil, paths, cfg, err
	}
	key, _, err := config.ResolveKey(deps.Env, cfg)
	if err != nil {
		return nil, paths, cfg, err
	}
	c := api.New(key, deps.Env)
	if deps.Transport != nil {
		c.HTTP.Transport = deps.Transport
	}
	if deps.Sleep != nil {
		c.Sleep = deps.Sleep
	}
	if debug, _ := cmd.Flags().GetBool("debug"); debug {
		c.Debug = deps.Stderr
	}
	return c, paths, cfg, nil
}

func newCatalog(cmd *cobra.Command, deps Deps) (*catalog.Service, error) {
	c, paths, err := newAPIClient(cmd, deps)
	if err != nil {
		return nil, err
	}
	return &catalog.Service{API: c, CacheDir: paths.Cache, Now: deps.Now}, nil
}

func newModelsCmd(deps Deps) *cobra.Command {
	models := &cobra.Command{Use: "models", Short: "Browse the kie.ai model catalog", RunE: groupRunE}
	models.AddCommand(newModelsListCmd(deps), newModelsSearchCmd(deps), newModelsPriceCmd(deps), newModelsHealthCmd(deps), newModelsSchemaCmd(deps))
	return models
}

func addFilterFlags(cmd *cobra.Command) {
	cmd.Flags().String("type", "", "task type substring (e.g. video)")
	cmd.Flags().String("provider", "", "provider name")
}

func emitModels(deps Deps, ms []catalog.Model) error {
	return output.Success(deps.Stdout, map[string]any{"total": len(ms), "models": ms})
}

func newModelsListCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List models (cached 1h)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, err := newCatalog(cmd, deps)
			if err != nil {
				return err
			}
			refresh, _ := cmd.Flags().GetBool("refresh")
			all, err := svc.List(cmd.Context(), refresh)
			if err != nil {
				return err
			}
			typ, _ := cmd.Flags().GetString("type")
			prov, _ := cmd.Flags().GetString("provider")
			return emitModels(deps, catalog.Filter(all, catalog.Query{Type: typ, Provider: prov}))
		},
	}
	addFilterFlags(cmd)
	cmd.Flags().Bool("refresh", false, "bypass the cache")
	return cmd
}

func newModelsSearchCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <text>",
		Short: "Search models by text in id, title and description",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newCatalog(cmd, deps)
			if err != nil {
				return err
			}
			all, err := svc.List(cmd.Context(), false)
			if err != nil {
				return err
			}
			typ, _ := cmd.Flags().GetString("type")
			prov, _ := cmd.Flags().GetString("provider")
			return emitModels(deps, catalog.Filter(all, catalog.Query{Type: typ, Provider: prov, Text: args[0]}))
		},
	}
	addFilterFlags(cmd)
	return cmd
}

func newModelsPriceCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "price <model>",
		Short: "Show the pricing description of a model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newCatalog(cmd, deps)
			if err != nil {
				return err
			}
			desc, err := svc.Price(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return output.Success(deps.Stdout, map[string]any{"model": args[0], "pricingDesc": desc})
		},
	}
}

func newModelsHealthCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "health <model>",
		Short: "Show recent success-rate points of a model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newCatalog(cmd, deps)
			if err != nil {
				return err
			}
			points, err := svc.Health(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return output.Success(deps.Stdout, map[string]any{"model": args[0], "points": json.RawMessage(points)})
		},
	}
}
