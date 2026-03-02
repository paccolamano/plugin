package plugincmd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"runtime"
	"strings"

	"github.com/paccolamano/plugin/pbplugin"
	"github.com/paccolamano/plugin/plugincmd/internal/git"

	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// pluginCollectionName is the system collection used to track installed plugins.
// It is non-modifiable from the admin UI (System: true).
const pluginCollectionName = "_pb_plugins"

// Config holds the plugin manager configuration.
type Config struct {
	// Dir is the directory where compiled plugin .so files are stored.
	// Defaults to pb_data/../pb_plugins.
	Dir string
}

func MustRegister(app core.App, rootCmd *cobra.Command, config Config) {
	if err := Register(app, rootCmd, config); err != nil {
		panic(err)
	}
}

func Register(app core.App, rootCmd *cobra.Command, config Config) error {
	if config.Dir == "" {
		config.Dir = filepath.Join(app.DataDir(), "../pb_plugins")
	}

	pm := &pluginCmd{app: app, config: config}

	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		if err := ensureCollection(e.App); err != nil {
			return err
		}

		return loadAll(e.App, pm.config.Dir)
	})

	if rootCmd != nil {
		rootCmd.AddCommand(pm.newCommand())
	}

	return nil
}

func ensureCollection(app core.App) error {
	if _, err := app.FindCollectionByNameOrId(pluginCollectionName); err == nil {
		return nil // already exists
	}

	pluginCollection := core.NewBaseCollection(pluginCollectionName)
	pluginCollection.System = true

	pluginCollection.Fields.Add(
		&core.TextField{
			Name:     "repository",
			Required: true,
		},
		&core.TextField{
			Name:     "version",
			Required: true,
		},
		&core.TextField{
			Name:     "file",
			Required: true,
		},
	)

	return app.Save(pluginCollection)
}

func loadAll(app core.App, dir string) error {
	if dir == "" {
		dir = filepath.Join(app.DataDir(), "../pb_plugins")
	}

	records, err := app.FindAllRecords(pluginCollectionName)
	if err != nil {
		return nil // collection does not exist yet
	}

	for _, record := range records {
		repo := record.GetString("repository")
		soPath := filepath.Join(dir, record.GetString("file"))

		p, err := plugin.Open(soPath)
		if err != nil {
			return fmt.Errorf("plugin %q: open failed: %w", repo, err)
		}

		sym, err := p.Lookup("Plugin")
		if err != nil {
			return fmt.Errorf("plugin %q: missing exported 'Plugin' symbol: %w", repo, err)
		}

		pbPlugin, ok := sym.(*pbplugin.PBPlugin)
		if !ok {
			return fmt.Errorf("plugin %q: 'Plugin' does not implement PBPlugin", repo)
		}

		if err := (*pbPlugin).Register(app); err != nil {
			return fmt.Errorf("plugin %q: Register failed: %w", repo, err)
		}
	}

	return nil
}

type pluginCmd struct {
	app    core.App
	config Config
}

func (pm *pluginCmd) newCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "plugin",
		Short:        "Manage PocketBase plugins",
		SilenceUsage: true,
	}

	cmd.AddCommand(pm.cmdInstall())
	cmd.AddCommand(pm.cmdRemove())
	cmd.AddCommand(pm.cmdList())

	return cmd
}

func (pm *pluginCmd) cmdInstall() *cobra.Command {
	var token, serverURL, provider string

	cmd := &cobra.Command{
		Use:          "install <owner/repo>",
		Short:        "Install a plugin from a Git hosting provider",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverURL != "" && !cmd.Flags().Changed("provider") {
				return fmt.Errorf("flag --provider is required when --url is specified (supported: github, gitea, forgejo, gitlab)")
			}
			return pm.install(cmd.Context(), args[0], provider, serverURL, token)
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "github", "git provider: github, gitea, forgejo, gitlab")
	cmd.Flags().StringVar(&serverURL, "url", "", "base URL of the Git server API for self-hosted instances")
	cmd.Flags().StringVar(&token, "token", "", "personal access token (required for private repositories)")

	return cmd
}

func (pm *pluginCmd) install(ctx context.Context, repo, provider, serverURL, token string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid repo format: expected owner/repo")
	}
	repoName := parts[1]

	// Check if already installed.
	if existing, err := pm.app.FindFirstRecordByData(pluginCollectionName, "repository", repo); err == nil {
		return fmt.Errorf("plugin %q is already installed (version %s)", repo, existing.GetString("version"))
	}

	gc, err := git.NewClient(provider, token, serverURL)
	if err != nil {
		return err
	}

	release, err := gc.GetRelease(ctx, repo, "latest")
	if err != nil {
		return err
	}

	// Download and extract source into a temp directory.
	tmpDir, err := os.MkdirTemp("", "pbplugin-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Printf("Downloading %s@%s...\n", repo, release.TagName)
	body, err := gc.DownloadRelease(ctx, release.TarballURL)
	if err != nil {
		return err
	}
	defer body.Close()

	srcDir, err := extractTarball(body, tmpDir)
	if err != nil {
		return err
	}

	// Compile the plugin locally (same Go version as host).
	if err := os.MkdirAll(pm.config.Dir, 0o755); err != nil {
		return fmt.Errorf("failed to create plugins directory: %w", err)
	}

	soName := fmt.Sprintf("%s_%s_%s.so", repoName, runtime.GOOS, runtime.GOARCH)
	destPath := filepath.Join(pm.config.Dir, soName)

	fmt.Printf("Compiling %s...\n", repo)
	if err := compilePlugin(srcDir, destPath); err != nil {
		return err
	}

	// Persist to the system collection.
	pluginCollection, err := pm.app.FindCollectionByNameOrId(pluginCollectionName)
	if err != nil {
		os.Remove(destPath)
		return fmt.Errorf("plugins collection not found: %w", err)
	}

	record := core.NewRecord(pluginCollection)
	record.Set("repository", repo)
	record.Set("version", release.TagName)
	record.Set("file", soName)

	if err := pm.app.Save(record); err != nil {
		os.Remove(destPath)
		return fmt.Errorf("failed to save plugin record: %w", err)
	}

	fmt.Printf("Installed %s@%s\n", repo, release.TagName)
	return nil
}

func (pm *pluginCmd) cmdRemove() *cobra.Command {
	return &cobra.Command{
		Use:          "rm <owner/repo>",
		Short:        "Remove an installed plugin",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pm.remove(args[0])
		},
	}
}

func (pm *pluginCmd) remove(repo string) error {
	record, err := pm.app.FindFirstRecordByData(pluginCollectionName, "repository", repo)
	if err != nil {
		return fmt.Errorf("plugin %q is not installed", repo)
	}

	soPath := filepath.Join(pm.config.Dir, record.GetString("file"))
	if err := os.Remove(soPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plugin file: %w", err)
	}

	if err := pm.app.Delete(record); err != nil {
		return fmt.Errorf("failed to remove plugin record: %w", err)
	}

	fmt.Printf("Removed %s\n", repo)
	return nil
}

func (pm *pluginCmd) cmdList() *cobra.Command {
	return &cobra.Command{
		Use:          "ls",
		Short:        "List installed plugins",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pm.list()
		},
	}
}

func (pm *pluginCmd) list() error {
	records, err := pm.app.FindAllRecords(pluginCollectionName)
	if err != nil || len(records) == 0 {
		fmt.Println("No plugins installed.")
		return nil
	}

	fmt.Printf("%-45s %s\n", "PLUGIN", "VERSION")
	fmt.Println(strings.Repeat("-", 60))
	for _, r := range records {
		fmt.Printf("%-45s %s\n", r.GetString("repository"), r.GetString("version"))
	}
	return nil
}

// extractTarball decomprime ed estrae un tarball gzip dallo stream r nella directory destDir.
// Restituisce il percorso della directory radice estratta.
func extractTarball(r io.Reader, destDir string) (string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", fmt.Errorf("failed to decompress tarball: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	safeBase := filepath.Clean(destDir) + string(filepath.Separator)
	var topDir string

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read tarball: %w", err)
		}

		// Skip PAX extended-header entries (e.g. pax_global_header);
		// they carry metadata only and have no real path on disk.
		if header.Typeflag == tar.TypeXGlobalHeader || header.Typeflag == tar.TypeXHeader {
			continue
		}

		// Sanitize path to prevent zip-slip attacks.
		cleanName := filepath.Clean(header.Name)
		destPath := filepath.Join(destDir, cleanName)
		if !strings.HasPrefix(destPath+string(filepath.Separator), safeBase) {
			continue // skip unsafe entries
		}

		if topDir == "" {
			topDir = strings.SplitN(cleanName, string(filepath.Separator), 2)[0]
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return "", fmt.Errorf("failed to create directory %q: %w", destPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return "", fmt.Errorf("failed to create parent directory: %w", err)
			}
			f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode)&0o755)
			if err != nil {
				return "", fmt.Errorf("failed to create file %q: %w", destPath, err)
			}
			_, copyErr := io.Copy(f, tr)
			f.Close()
			if copyErr != nil {
				return "", fmt.Errorf("failed to write file %q: %w", destPath, copyErr)
			}
		}
	}

	if topDir == "" {
		return "", fmt.Errorf("empty or invalid source tarball")
	}

	return filepath.Join(destDir, topDir), nil
}

func compilePlugin(srcDir, outputPath string) error {
	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", outputPath, ".")
	cmd.Dir = srcDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("compilation failed: %w\n%s", err, msg)
		}
		return fmt.Errorf("compilation failed: %w", err)
	}
	return nil
}
