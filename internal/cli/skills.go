package cli

import (
	"path/filepath"

	kiecli "github.com/mik3dev/kie-cli"
	"github.com/mik3dev/kie-cli/internal/config"
	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/mik3dev/kie-cli/internal/skills"
	"github.com/spf13/cobra"
)

const bundledSkill = "skills/kie-generate"

func newSkillsCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Manage the bundled agent skill",
		RunE:  groupRunE,
	}
	install := &cobra.Command{
		Use:   "install",
		Short: "Install the kie-generate skill (default ~/.claude/skills)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target, _ := cmd.Flags().GetString("target")
			if target == "" {
				home := config.UserHome(deps.Env)
				if home == "" {
					return usageErr("cannot find the home directory: set HOME or USERPROFILE, or pass --target")
				}
				target = filepath.Join(home, ".claude", "skills")
			}
			files, err := skills.Install(kiecli.Assets, bundledSkill, target)
			if err != nil {
				return &output.Error{Code: output.CodeIO, Message: err.Error()}
			}
			return output.Success(deps.Stdout, map[string]any{"target": target, "files": files})
		},
	}
	install.Flags().String("target", "", "skills directory (default <home>/.claude/skills)")
	cmd.AddCommand(install)
	return cmd
}
