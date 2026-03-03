package brew

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fastbrew/internal/progress"
)

type TapFormulaInstaller struct {
	client   *Client
	resolver *TapFormulaResolver
}

func NewTapFormulaInstaller(c *Client, tm *TapManager) *TapFormulaInstaller {
	return &TapFormulaInstaller{
		client:   c,
		resolver: NewTapFormulaResolver(tm),
	}
}

func (i *TapFormulaInstaller) InstallTapFormula(ref string, opts InstallOptions) error {
	resolved, err := i.resolver.Resolve(ref)
	if err != nil {
		if opts.StrictNative {
			return err
		}
		return i.fallbackToBrew(ref, err)
	}

	meta, err := ParseTapFormula(resolved.FormulaPath)
	if err != nil {
		if opts.StrictNative {
			return err
		}
		return i.fallbackToBrew(ref, err)
	}

	if len(meta.UnsupportedStanzas) > 0 {
		if opts.StrictNative {
			return &UnsupportedError{
				Formula: ref,
				Stanzas: meta.UnsupportedStanzas,
			}
		}
		return i.fallbackToBrewWithUnsupported(ref, meta)
	}

	if err := i.installDependencies(meta, opts); err != nil {
		return err
	}

	installed, err := i.checkInstalled(resolved.Name, meta.Version)
	if err != nil {
		return err
	}
	if installed {
		fmt.Printf("  ✅ %s is already installed\n", resolved.Name)
		return nil
	}

	if err := i.downloadAndInstall(meta, resolved); err != nil {
		if opts.StrictNative {
			return err
		}
		return i.fallbackToBrew(ref, err)
	}

	return i.linkFormula(resolved.Name, meta.Version)
}

func (i *TapFormulaInstaller) installDependencies(meta *TapFormulaMetadata, opts InstallOptions) error {
	for _, dep := range meta.RuntimeDeps {
		cellarPath := filepath.Join(i.client.Cellar, dep)
		if _, err := os.Stat(cellarPath); os.IsNotExist(err) {
			if err := i.client.InstallNativeWithOptions([]string{dep}, opts); err != nil {
				return fmt.Errorf("failed to install dependency %s: %w", dep, err)
			}
		}
	}
	return nil
}

func (i *TapFormulaInstaller) checkInstalled(name, version string) (bool, error) {
	cellarPath := filepath.Join(i.client.Cellar, name, version)
	_, err := os.Stat(cellarPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (i *TapFormulaInstaller) downloadAndInstall(meta *TapFormulaMetadata, resolved *ResolvedFormula) error {
	url, ok := GetBottleURL(meta, meta.Version)
	if !ok || url == "" {
		return fmt.Errorf("no bottle URL available for %s", meta.Name)
	}

	sha, ok := meta.SHA256s[GetOSFromMeta(meta)]
	if !ok {
		return fmt.Errorf("no SHA256 found for %s", meta.Name)
	}

	tarPath, err := i.downloadTapBottle(url, sha, meta.Name)
	if err != nil {
		return err
	}

	cellarPath := filepath.Join(i.client.Cellar, meta.Name)
	if err := os.MkdirAll(cellarPath, 0755); err != nil {
		return fmt.Errorf("failed to create cellar path: %w", err)
	}

	versionPath := filepath.Join(cellarPath, meta.Version)
	if err := os.MkdirAll(versionPath, 0755); err != nil {
		return fmt.Errorf("failed to create version path: %w", err)
	}

	if err := ExtractTapArchive(tarPath, versionPath, i.client.Prefix); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	return i.stageFiles(meta, versionPath)
}

func (i *TapFormulaInstaller) downloadTapBottle(url, sha256, name string) (string, error) {
	cacheDir, _ := i.client.GetCacheDir()
	tarPath := filepath.Join(cacheDir, fmt.Sprintf("%s-tap.bottle", name))

	var tracker progress.ProgressTracker
	if i.client.ProgressManager != nil {
		tracker = i.client.ProgressManager.Register(name, url)
		defer i.client.ProgressManager.Unregister(name)
	}

	if err := i.client.DownloadWithProgress(url, tarPath, sha256, tracker); err != nil {
		return "", err
	}

	return tarPath, nil
}

func (i *TapFormulaInstaller) stageFiles(meta *TapFormulaMetadata, versionPath string) error {
	stage := func(directives []InstallDirective, destSubdir string) error {
		for _, dir := range directives {
			src := filepath.Join(versionPath, dir.Source)
			dest := filepath.Join(versionPath, destSubdir, filepath.Base(dir.Destination))

			if _, err := os.Stat(src); err != nil {
				continue
			}

			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}

			if rename, ok := dir.Renames[filepath.Base(src)]; ok {
				dest = filepath.Join(filepath.Dir(dest), rename)
			}

			if err := os.Rename(src, dest); err != nil {
				return err
			}
		}
		return nil
	}

	if err := stage(meta.BinFiles, "bin"); err != nil {
		return err
	}
	if err := stage(meta.SbinFiles, "sbin"); err != nil {
		return err
	}
	if err := stage(meta.LibexecFiles, "libexec"); err != nil {
		return err
	}
	if err := stage(meta.BashCompletions, "etc/bash_completion.d"); err != nil {
		return err
	}
	if err := stage(meta.ZshCompletions, "share/zsh/site-functions"); err != nil {
		return err
	}
	if err := stage(meta.FishCompletions, "share/fish/vendor_completions.d"); err != nil {
		return err
	}
	if err := stage(meta.ManPages, "share/man"); err != nil {
		return err
	}

	return nil
}

func (i *TapFormulaInstaller) linkFormula(name, version string) error {
	result, err := i.client.Link(name, version)
	if err != nil {
		return err
	}
	if !result.Success {
		fmt.Printf("  ⚠️  Link completed with errors for %s\n", name)
	}
	return nil
}

func (i *TapFormulaInstaller) fallbackToBrew(ref string, cause error) error {
	fmt.Printf("  ⚠️  Falling back to brew for %s: %v\n", ref, cause)
	ref = strings.TrimPrefix(ref, "homebrew/")
	return i.client.InstallBrewFallback(ref)
}

func (i *TapFormulaInstaller) fallbackToBrewWithUnsupported(ref string, meta *TapFormulaMetadata) error {
	fmt.Printf("  ⚠️  Falling back to brew for %s (unsupported stanzas: %v)\n", ref, meta.UnsupportedStanzas)
	ref = strings.TrimPrefix(ref, "homebrew/")
	return i.client.InstallBrewFallback(ref)
}

func (c *Client) InstallBrewFallback(ref string) error {
	return fmt.Errorf("brew fallback not implemented: %s", ref)
}
