package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "env",
		Aliases: []string{"envs", "secrets"},
		Short:   "Manage a project's environment variables",
		Long: `Manage a project's environment variables (secrets).

Values are encrypted at rest. Changes apply on the next deployment — run
"brimble deploy" afterwards.`,
	}

	cmd.AddCommand(newEnvListCmd())
	cmd.AddCommand(newEnvSetCmd())
	cmd.AddCommand(newEnvDeleteCmd())
	cmd.AddCommand(newEnvPullCmd())
	cmd.AddCommand(newEnvPushCmd())
	return cmd
}

func addEnvironmentFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVarP(target, "environment", "e", "", "Scope: production or preview (default: production)")
}

// envScope maps the --environment flag to Core's PRODUCTION / PREVIEW.
func envScope(flag string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(flag)) {
	case "":
		return "", nil
	case "production", "prod":
		return "PRODUCTION", nil
	case "preview":
		return "PREVIEW", nil
	}
	return "", fmt.Errorf("--environment must be production or preview, got %q", flag)
}

func newEnvListCmd() *cobra.Command {
	var environment string
	var reveal bool

	cmd := &cobra.Command{
		Use:     "list [project]",
		Aliases: []string{"ls"},
		Short:   "List environment variables",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				scope, err := envScope(environment)
				if err != nil {
					return err
				}
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				envs, err := loadEnvEntries(ctx, ref, scope)
				if err != nil {
					return err
				}
				if reveal {
					if err := decryptEntries(ctx, envs); err != nil {
						return err
					}
				}
				if jsonOutput() {
					return printResult(envJSON(envs, reveal))
				}
				printEnvList(ref.Display(), envs, reveal)
				return nil
			})
		},
	}

	addEnvironmentFlag(cmd, &environment)
	cmd.Flags().BoolVar(&reveal, "reveal", false, "Show decrypted values")
	return cmd
}

func newEnvSetCmd() *cobra.Command {
	var environment, name, value, file string

	cmd := &cobra.Command{
		Use:   "set [project] KEY=VALUE...",
		Short: "Create or update environment variables",
		Long: `Create or update environment variables.

Pass KEY=VALUE pairs. Pass a bare KEY to be prompted for its value with
hidden input, which keeps secrets out of your shell history.`,
		Example: `  brimble env set API_URL=https://api.example.com DEBUG=false
  brimble env set my-api STRIPE_KEY        # prompts for the value
  brimble env set -e preview FEATURE_X=on`,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				scope, err := envScope(environment)
				if err != nil {
					return err
				}

				projectArg, pairs := splitProjectAndPairs(args)
				if name != "" {
					if cmd.Flags().Changed("value") {
						pairs = append(pairs, name+"="+value)
					} else {
						pairs = append(pairs, name)
					}
				}

				var vars []client.EnvVar
				if file != "" {
					vars, err = readDotenv(file)
					if err != nil {
						return err
					}
				}
				// Bare KEY arguments get their value from a hidden prompt.
				var prompt []int
				for _, p := range pairs {
					v, hasValue, err := parsePair(p)
					if err != nil {
						return err
					}
					if !hasValue {
						prompt = append(prompt, len(vars))
					}
					vars = append(vars, v)
				}
				if len(vars) == 0 {
					if !ui.IsInteractive() {
						return fmt.Errorf("pass KEY=VALUE pairs to set")
					}
					key, err := ui.Text("Variable name", "API_URL", "")
					if err != nil {
						return err
					}
					prompt = append(prompt, 0)
					vars = append(vars, client.EnvVar{Name: strings.TrimSpace(key)})
				}
				for _, v := range vars {
					if !validEnvName(v.Name) {
						return fmt.Errorf("%q is not a valid variable name (letters, digits and _; must not start with a digit)", v.Name)
					}
				}
				for _, i := range prompt {
					if !ui.IsInteractive() {
						return fmt.Errorf("no value for %s — pass %s=VALUE", vars[i].Name, vars[i].Name)
					}
					vars[i].Value, err = ui.Password("Value for "+vars[i].Name, "")
					if err != nil {
						return err
					}
				}

				ref, err := pickProject(ctx, projectArg)
				if err != nil {
					return err
				}
				created, updated, err := upsertEnvVars(ctx, ref, scope, vars)
				if err != nil {
					return err
				}
				return reportEnvChanges(ref, created, updated)
			})
		},
	}

	addEnvironmentFlag(cmd, &environment)
	cmd.Flags().StringVar(&name, "name", "", "Variable name (alternative to KEY=VALUE)")
	cmd.Flags().StringVar(&value, "value", "", "Variable value (used with --name)")
	cmd.Flags().StringVarP(&file, "file", "f", "", "Read variables from a .env file")
	return cmd
}

func newEnvDeleteCmd() *cobra.Command {
	var environment string
	var yes bool

	cmd := &cobra.Command{
		Use:     "delete [project] [NAME...]",
		Aliases: []string{"rm", "unset"},
		Short:   "Delete environment variables",
		Long: `Delete environment variables.

In a linked directory, every argument is a variable name. Otherwise the first
argument is the project. With no names you pick from a list.`,
		Example: `  brimble env delete OLD_TOKEN
  brimble env delete my-api OLD_TOKEN LEGACY_URL -y`,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				scope, err := envScope(environment)
				if err != nil {
					return err
				}
				projectArg, names := "", args
				if linked == nil && len(args) > 0 {
					projectArg, names = args[0], args[1:]
				}
				ref, err := pickProject(ctx, projectArg)
				if err != nil {
					return err
				}
				envs, err := loadEnvEntries(ctx, ref, scope)
				if err != nil {
					return err
				}
				envs = ownEntries(envs)
				if len(envs) == 0 {
					return fmt.Errorf("%s has no environment variables", ref.Display())
				}

				targets, err := pickEnvEntries(envs, names)
				if err != nil {
					return err
				}
				labels := make([]string, len(targets))
				for i, t := range targets {
					labels[i] = t.Name
				}
				ok, err := confirmOrAbort(yes, fmt.Sprintf("Delete %s from %s?", strings.Join(labels, ", "), ref.Display()), true)
				if err != nil || !ok {
					return err
				}
				for _, t := range targets {
					err = ui.WithSpinner("Deleting "+t.Name+"…", func() error {
						return coreClient.DeleteEnvVar(ctx, ref.ID, t.ID, cfg.TeamID)
					})
					if err != nil {
						return fmt.Errorf("delete %s: %w", t.Name, err)
					}
					ui.Success("Deleted %s", t.Name)
				}
				ui.Hint("Run `brimble deploy` to apply the change")
				return nil
			})
		},
	}

	addEnvironmentFlag(cmd, &environment)
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newEnvPullCmd() *cobra.Command {
	var environment, file string
	var yes bool

	cmd := &cobra.Command{
		Use:   "pull [project]",
		Short: "Download environment variables into a .env file",
		Example: `  brimble env pull
  brimble env pull my-api -f .env.production
  brimble env pull -f -        # print to stdout`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				scope, err := envScope(environment)
				if err != nil {
					return err
				}
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				envs, err := loadEnvEntries(ctx, ref, scope)
				if err != nil {
					return err
				}
				if scope == "" {
					envs = productionEntries(envs)
				}
				if err := decryptEntries(ctx, envs); err != nil {
					return err
				}
				vars := make([]client.EnvVar, 0, len(envs))
				for _, e := range envs {
					vars = append(vars, client.EnvVar{Name: e.Name, Value: e.Value})
				}
				content := formatDotenv(vars)

				if file == "-" {
					_, err := os.Stdout.WriteString(content)
					return err
				}
				if _, err := os.Stat(file); err == nil {
					ok, err := confirmOrAbort(yes, fmt.Sprintf("Overwrite %s?", file), true)
					if err != nil || !ok {
						return err
					}
				}
				if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
					return err
				}
				ui.Success("Wrote %d variables from %s to %s", len(vars), ref.Display(), file)
				ui.Hint("Keep %s out of version control", file)
				return nil
			})
		},
	}

	addEnvironmentFlag(cmd, &environment)
	cmd.Flags().StringVarP(&file, "file", "f", ".env", "File to write (- for stdout)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Overwrite without asking")
	return cmd
}

func newEnvPushCmd() *cobra.Command {
	var environment, file string
	var yes bool

	cmd := &cobra.Command{
		Use:     "push [project]",
		Short:   "Upload variables from a .env file",
		Long:    "Create or update every variable in a .env file. Variables not in the file are left alone.",
		Example: "  brimble env push -f .env.production",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				scope, err := envScope(environment)
				if err != nil {
					return err
				}
				vars, err := readDotenv(file)
				if err != nil {
					return err
				}
				if len(vars) == 0 {
					return fmt.Errorf("%s has no variables", file)
				}
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				ok, err := confirmOrAbort(yes, fmt.Sprintf("Upload %d variables from %s to %s?", len(vars), file, ref.Display()), false)
				if err != nil || !ok {
					return err
				}
				created, updated, err := upsertEnvVars(ctx, ref, scope, vars)
				if err != nil {
					return err
				}
				return reportEnvChanges(ref, created, updated)
			})
		},
	}

	addEnvironmentFlag(cmd, &environment)
	cmd.Flags().StringVarP(&file, "file", "f", ".env", "File to read")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func loadEnvEntries(ctx context.Context, ref ProjectRef, scope string) ([]client.Env, error) {
	var list client.EnvList
	err := ui.WithSpinner("Loading environment variables…", func() error {
		var err error
		list, err = coreClient.ListEnvVars(ctx, ref.ID, scope, cfg.TeamID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return envEntries(list), nil
}

func envEntries(list client.EnvList) []client.Env {
	out := make([]client.Env, 0, len(list.Envs))
	for _, e := range list.Envs {
		if e.Name == "" {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type envJSONRow struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Environment string `json:"environment"`
	UpdatedAt   string `json:"updatedAt"`
	Source      string `json:"source"`
	Value       string `json:"value,omitempty"`
}

func envJSON(envs []client.Env, reveal bool) []envJSONRow {
	out := make([]envJSONRow, 0, len(envs))
	for _, e := range envs {
		row := envJSONRow{ID: e.ID, Name: e.Name, Environment: e.Environment, UpdatedAt: e.UpdatedAt, Source: e.Source}
		if reveal {
			row.Value = e.Value
		}
		out = append(out, row)
	}
	return out
}

// ownEntries drops variables inherited from the project's environment; those
// are managed with "brimble environment", not on the project.
func ownEntries(envs []client.Env) []client.Env {
	var out []client.Env
	for _, e := range envs {
		if e.Source != "environment" && e.ID != "" {
			out = append(out, e)
		}
	}
	return out
}

func productionEntries(envs []client.Env) []client.Env {
	var out []client.Env
	for _, e := range envs {
		if e.Environment == "" || strings.EqualFold(e.Environment, "PRODUCTION") || e.Source == "environment" {
			out = append(out, e)
		}
	}
	return out
}

func decryptEntries(ctx context.Context, envs []client.Env) error {
	if len(envs) == 0 {
		return nil
	}
	in := make([]client.EnvVar, len(envs))
	for i, e := range envs {
		in[i] = client.EnvVar{Name: e.Name, Value: e.Value}
	}
	var out []client.EnvVar
	err := ui.WithSpinner("Decrypting values…", func() error {
		var err error
		out, err = coreClient.Decrypt(ctx, in, cfg.TeamID)
		return err
	})
	if err != nil {
		return err
	}
	if len(out) != len(envs) {
		return fmt.Errorf("decrypt returned %d values for %d variables", len(out), len(envs))
	}
	for i := range envs {
		envs[i].Value = out[i].Value
	}
	return nil
}

// upsertEnvVars updates variables that already exist in scope and creates the rest.
func upsertEnvVars(ctx context.Context, ref ProjectRef, scope string, vars []client.EnvVar) (created, updated []string, err error) {
	existing, err := loadEnvEntries(ctx, ref, scope)
	if err != nil {
		return nil, nil, err
	}
	byName := map[string]client.Env{}
	for _, e := range ownEntries(existing) {
		if scope == "" && e.Environment != "" && !strings.EqualFold(e.Environment, "PRODUCTION") {
			continue
		}
		byName[e.Name] = e
	}

	var toCreate []client.EnvVar
	seen := map[string]bool{}
	for _, v := range vars {
		if seen[v.Name] {
			continue
		}
		seen[v.Name] = true
		if e, ok := byName[v.Name]; ok {
			err := ui.WithSpinner("Updating "+v.Name+"…", func() error {
				return coreClient.UpdateEnvVar(ctx, ref.ID, e.ID, v, cfg.TeamID)
			})
			if err != nil {
				return created, updated, fmt.Errorf("update %s: %w", v.Name, err)
			}
			updated = append(updated, v.Name)
			continue
		}
		toCreate = append(toCreate, v)
	}

	if len(toCreate) > 0 {
		err := ui.WithSpinner(fmt.Sprintf("Creating %d variable(s)…", len(toCreate)), func() error {
			return coreClient.CreateEnvVars(ctx, ref.ID, scope, toCreate, cfg.TeamID)
		})
		if err != nil {
			return created, updated, err
		}
		for _, v := range toCreate {
			created = append(created, v.Name)
		}
	}
	return created, updated, nil
}

type envChangeResult struct {
	Project string   `json:"project"`
	Created []string `json:"created"`
	Updated []string `json:"updated"`
}

func reportEnvChanges(ref ProjectRef, created, updated []string) error {
	if created == nil {
		created = []string{}
	}
	if updated == nil {
		updated = []string{}
	}
	if jsonOutput() {
		return printResult(envChangeResult{Project: ref.Name, Created: created, Updated: updated})
	}
	if len(created) > 0 {
		ui.Success("Added %s to %s", strings.Join(created, ", "), ref.Display())
	}
	if len(updated) > 0 {
		ui.Success("Updated %s on %s", strings.Join(updated, ", "), ref.Display())
	}
	ui.Hint("Run `brimble deploy` to apply the change")
	return nil
}

func pickEnvEntries(envs []client.Env, names []string) ([]client.Env, error) {
	if len(names) > 0 {
		var out []client.Env
		for _, n := range names {
			found := false
			for _, e := range envs {
				if e.Name == n || e.ID == n {
					out = append(out, e)
					found = true
				}
			}
			if !found {
				return nil, fmt.Errorf("variable %q not found", n)
			}
		}
		return out, nil
	}
	if !ui.IsInteractive() {
		return nil, fmt.Errorf("variable name is required")
	}
	opts := make([]ui.Option, 0, len(envs))
	byID := map[string]client.Env{}
	for _, e := range envs {
		label := e.Name
		if e.Environment != "" {
			label += ui.Dim("  " + strings.ToLower(e.Environment))
		}
		opts = append(opts, ui.Option{Label: label, Value: e.ID})
		byID[e.ID] = e
	}
	id, err := ui.Select("Select a variable", opts)
	if err != nil {
		return nil, err
	}
	return []client.Env{byID[id]}, nil
}

func printEnvList(project string, envs []client.Env, reveal bool) {
	ui.Title(project + " · environment variables")
	if len(envs) == 0 {
		ui.Muted("  No variables yet — add one with `brimble env set KEY=VALUE`")
		return
	}
	headers := []string{"NAME", "SCOPE", "UPDATED"}
	if reveal {
		headers = append(headers, "VALUE")
	}
	rows := make([][]string, 0, len(envs))
	for _, e := range envs {
		scope := strings.ToLower(e.Environment)
		if e.Source == "environment" {
			scope = "shared"
		}
		row := []string{e.Name, scope, formatAgo(e.UpdatedAt)}
		if reveal {
			row = append(row, e.Value)
		}
		rows = append(rows, row)
	}
	ui.Table(headers, rows)
	fmt.Fprintln(os.Stdout)
	if !reveal {
		ui.Muted("  %d variable(s) · values hidden (--reveal to show)", len(envs))
	}
}

// splitProjectAndPairs separates KEY=VALUE arguments from an optional project.
// A bare KEY is a variable when a project is linked or a project was already given.
func splitProjectAndPairs(args []string) (project string, pairs []string) {
	for _, a := range args {
		if strings.Contains(a, "=") || project != "" || linked != nil {
			pairs = append(pairs, a)
			continue
		}
		project = a
	}
	// "brimble env set my-api" with nothing else: my-api is the project.
	return project, pairs
}

func parsePair(p string) (v client.EnvVar, hasValue bool, err error) {
	name, value, hasValue := strings.Cut(p, "=")
	name = strings.TrimSpace(name)
	if name == "" {
		return client.EnvVar{}, false, fmt.Errorf("invalid variable %q — use KEY=VALUE", p)
	}
	return client.EnvVar{Name: name, Value: value}, hasValue, nil
}

func validEnvName(s string) bool {
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		return false
	}
	for _, r := range s {
		if !(r == '_' || r == '.' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// readDotenv parses KEY=VALUE lines, ignoring blanks, comments and "export ".
// Double-quoted values support \n, \t, \" and \\ escapes; single quotes are literal.
func readDotenv(path string) ([]client.EnvVar, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s not found", path)
		}
		return nil, err
	}
	return parseDotenv(string(b))
}

func parseDotenv(content string) ([]client.EnvVar, error) {
	var out []client.EnvVar
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", i+1)
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)

		switch {
		case strings.HasPrefix(value, `"`):
			// Allow multi-line double-quoted values.
			raw := value[1:]
			for !closesQuote(raw) && i+1 < len(lines) {
				i++
				raw += "\n" + lines[i]
			}
			if !closesQuote(raw) {
				return nil, fmt.Errorf("line %d: unterminated quote for %s", i+1, name)
			}
			raw = raw[:strings.LastIndex(raw, `"`)]
			value = unescapeDouble(raw)
		case strings.HasPrefix(value, `'`):
			end := strings.LastIndex(value, `'`)
			if end == 0 {
				return nil, fmt.Errorf("line %d: unterminated quote for %s", i+1, name)
			}
			value = value[1:end]
		default:
			if j := strings.Index(value, " #"); j >= 0 {
				value = strings.TrimSpace(value[:j])
			}
		}
		out = append(out, client.EnvVar{Name: name, Value: value})
	}
	return out, nil
}

func closesQuote(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return true
		}
	}
	return false
}

func unescapeDouble(s string) string {
	r := strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\"`, `"`, `\\`, `\`)
	return r.Replace(s)
}

func formatDotenv(vars []client.EnvVar) string {
	var b strings.Builder
	for _, v := range vars {
		b.WriteString(v.Name)
		b.WriteByte('=')
		if v.Value == "" || strings.ContainsAny(v.Value, " \t\n\"'#$\\=") {
			r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)
			b.WriteString(`"` + r.Replace(v.Value) + `"`)
		} else {
			b.WriteString(v.Value)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
