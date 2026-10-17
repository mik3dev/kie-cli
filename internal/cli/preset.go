package cli

import (
	"fmt"
	"strings"

	kiecli "github.com/mik3dev/kie-cli"
	"github.com/mik3dev/kie-cli/internal/config"
	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/mik3dev/kie-cli/internal/preset"
	"github.com/spf13/cobra"
)

const factoryPresetsFile = "presets/default.yaml"

// presetRef is the preset a run was invoked with; the zero value means none.
type presetRef struct {
	name   string
	params map[string]any // read-only: shared with the loaded preset
}

func newPresetCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preset",
		Short: "List and show model presets (factory and ~/.kie-cli/presets.yaml)",
		RunE:  groupRunE,
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List the available presets",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			presets, err := loadPresets(deps)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(presets))
			for i, p := range presets {
				rows[i] = map[string]any{"name": p.Name, "description": p.Description, "model": p.Model, "source": p.Source}
			}
			return output.Success(deps.Stdout, map[string]any{"presets": rows})
		},
	}
	show := &cobra.Command{
		Use:   "show <name>",
		Short: "Show one preset with its params",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			p, err := lookupPreset(deps, args[0])
			if err != nil {
				return err
			}
			return output.Success(deps.Stdout, map[string]any{"preset": p})
		},
	}
	cmd.AddCommand(list, show)
	return cmd
}

// loadPresets merges the embedded factory presets with the user file.
func loadPresets(deps Deps) ([]preset.Preset, error) {
	factory, err := kiecli.Assets.ReadFile(factoryPresetsFile)
	if err != nil {
		return nil, &output.Error{Code: output.CodeIO, Message: fmt.Sprintf("read embedded presets: %v", err)}
	}
	paths, err := config.ResolvePaths(deps.Env)
	if err != nil {
		return nil, err
	}
	presets, err := preset.Load(factory, paths.Presets)
	if err != nil {
		return nil, usageErr("%v", err)
	}
	return presets, nil
}

// lookupPreset finds a preset by name, with or without the leading '@'.
func lookupPreset(deps Deps, name string) (preset.Preset, error) {
	name = strings.TrimPrefix(name, "@")
	presets, err := loadPresets(deps)
	if err != nil {
		return preset.Preset{}, err
	}
	p, ok := preset.Find(presets, name)
	if !ok {
		return preset.Preset{}, usageErr("unknown preset @%s; see kie-cli preset list", name)
	}
	return p, nil
}
