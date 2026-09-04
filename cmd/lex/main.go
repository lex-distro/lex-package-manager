// Command lex is the portable LFS/BLFS package manager frontend.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/mikuripa/lex/internal/conf"
	"github.com/mikuripa/lex/internal/db"
	"github.com/mikuripa/lex/internal/install"
	"github.com/mikuripa/lex/internal/recipe"
	"github.com/mikuripa/lex/internal/remove"
	"github.com/mikuripa/lex/internal/repo"
	"github.com/mikuripa/lex/internal/root"
)

const version = "0.1.0"

const helpText = `lex %s - portable LFS/BLFS package manager

usage: lex <command> [args]

commands:
  sync              sync recipe repos (git pull/clone)
  install <pkg|@group> [--method=binary|source]
                    install package or group
  remove <pkg|@group>
                    remove installed package or group
  info <pkg> [--method=binary|source] [--lexbuild|--installed]
                    show recipe / installed info (default: combined,
                    or InfoMode from lex.conf)
  list              list installed packages
  verify <pkg|@group>
                    verify installed files

options:
  -h, --help        show this help
  -v, --version     show version
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lex: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, helpText, version)
		return fmt.Errorf("no command given")
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Printf(helpText, version)
		return nil
	case "-v", "--version", "version":
		fmt.Println("lex " + version)
		return nil
	case "info":
		return cmdInfo(args[1:])
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("usage: lex list")
		}
		return cmdList()
	case "sync":
		if len(args) != 1 {
			return fmt.Errorf("usage: lex sync")
		}
		return cmdSync()
	case "install":
		return cmdInstall(args[1:])
	case "remove":
		return cmdRemove(args[1:])
	case "verify":
		return cmdVerify(args[1:])
	default:
		fmt.Fprintf(os.Stderr, helpText, version)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func loadConf(lexRoot string) (conf.Config, error) {
	path := filepath.Join(lexRoot, "lex.conf")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return conf.Default(), nil
	}
	return conf.ParseFile(path)
}

// flags win over InfoMode
func cmdInfo(args []string) error {
	var pkg, methodOverride string
	var wantLexbuild, wantInstalled bool
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--method="):
			methodOverride = strings.TrimPrefix(a, "--method=")
			if methodOverride != "binary" && methodOverride != "source" {
				return fmt.Errorf("invalid --method %q: want binary|source", methodOverride)
			}
		case a == "--lexbuild":
			wantLexbuild = true
		case a == "--installed":
			wantInstalled = true
		case strings.HasPrefix(a, "--"):
			return fmt.Errorf("unknown flag %q", a)
		default:
			if pkg != "" {
				return fmt.Errorf("usage: lex info <pkg> [--method=binary|source] [--lexbuild|--installed]")
			}
			pkg = a
		}
	}
	if pkg == "" {
		return fmt.Errorf("usage: lex info <pkg> [--method=binary|source] [--lexbuild|--installed]")
	}
	if wantLexbuild && wantInstalled {
		return fmt.Errorf("flags --lexbuild and --installed are mutually exclusive")
	}
	if err := repo.ValidateName(pkg); err != nil {
		return err
	}
	lexRoot, err := root.Resolve()
	if err != nil {
		return err
	}
	cfg, err := loadConf(lexRoot)
	if err != nil {
		return err
	}
	prefer := cfg.Prefer
	if methodOverride != "" {
		prefer = methodOverride
	}

	showRecipe := true
	showInstalled := true
	switch {
	case wantLexbuild:
		showInstalled = false
	case wantInstalled:
		showRecipe = false
	case cfg.InfoMode == "installed":
		showRecipe = false
	}

	var sb strings.Builder

	if showRecipe {
		r, src, err := repo.Find(lexRoot, cfg.Repos, pkg)
		if err != nil {
			// no recipe around (not synced or pruned): show installed
			// state instead, unless --lexbuild asked for the recipe
			if !showInstalled {
				return err
			}
			if _, derr := db.Load(lexRoot, pkg); derr == nil {
				sb.WriteString("Recipe : not found\n")
				showRecipe = false
			} else {
				return err
			}
		} else {
			method, err := r.SelectMethod(prefer)
			if err != nil {
				return err
			}
			writeRecipeInfo(&sb, r, src, method, prefer)
		}
	}

	if showInstalled {
		ins, err := db.Load(lexRoot, pkg)
		if err == nil {
			writeInstalledInfo(&sb, ins)
		} else if !os.IsNotExist(err) {
			return err
		} else if showRecipe {
			sb.WriteString("Installed : no\n")
		} else {
			return fmt.Errorf("package %q is not installed", pkg)
		}
	}
	fmt.Print(sb.String())
	return nil
}

func writeRecipeInfo(sb *strings.Builder, r *recipe.Recipe, src, method, prefer string) {
	fmt.Fprintf(sb, "\nName : %s\n", r.Name)
	fmt.Fprintf(sb, "Version : %s\n", r.Version)
	if r.Homepage != "" {
		fmt.Fprintf(sb, "Homepage : %s\n", r.Homepage)
	}
	if r.Description != "" {
		fmt.Fprintf(sb, "Description : %s\n", r.Description)
	}
	if len(r.Deps) > 0 {
		fmt.Fprintf(sb, "Deps : %s\n", strings.Join(r.Deps, ", "))
	}
	fmt.Fprintf(sb, "Recipe : %s\n", src)
	fmt.Fprintf(sb, "Method : %s (prefer=%s)\n", method, prefer)
	if !r.SupportsArch(runtime.GOARCH) {
		fmt.Fprintf(sb, "  [warning: recipe arch %v does not include %s]\n", r.Arch, runtime.GOARCH)
	}
	switch method {
	case "binary":
		if r.Binary != nil {
			fmt.Fprintf(sb, "Source : %s\n", r.Binary.URL)
		}
	case "source":
		if r.Source != nil {
			fmt.Fprintf(sb, "Source : %s\n", r.Source.URL)
		}
	}
}

func writeInstalledInfo(sb *strings.Builder, ins *db.Installed) {
	fmt.Fprintf(sb, "Installed : %s via %s (%d files)\n", ins.Version, ins.Method, len(ins.Files))
}

func cmdInstall(args []string) error {
	var target, methodOverride string
	for _, a := range args {
		if strings.HasPrefix(a, "--method=") {
			methodOverride = strings.TrimPrefix(a, "--method=")
			if methodOverride != "binary" && methodOverride != "source" {
				return fmt.Errorf("invalid --method %q: want binary|source", methodOverride)
			}
			continue
		}
		if strings.HasPrefix(a, "--") {
			return fmt.Errorf("unknown flag %q", a)
		}
		if target != "" {
			return fmt.Errorf("usage: lex install <pkg|@group> [--method=binary|source]")
		}
		target = a
	}
	if target == "" {
		return fmt.Errorf("usage: lex install <pkg|@group> [--method=binary|source]")
	}
	lexRoot, err := root.Resolve()
	if err != nil {
		return err
	}
	cfg, err := loadConf(lexRoot)
	if err != nil {
		return err
	}
	names := []string{target}
	if strings.HasPrefix(target, "@") {
		names, err = repo.ExpandGroup(lexRoot, cfg.Repos, target)
		if err != nil {
			return err
		}
		if len(names) == 0 {
			return fmt.Errorf("group %q is empty", target)
		}
	}
	for _, n := range names {
		if err := install.Install(lexRoot, cfg, n, methodOverride); err != nil {
			return fmt.Errorf("%s: %w", n, err)
		}
	}
	return nil
}

func singleTarget(args []string, usage string) (string, error) {
	var target string
	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			return "", fmt.Errorf("unknown flag %q", a)
		}
		if target != "" {
			return "", fmt.Errorf("usage: lex %s", usage)
		}
		target = a
	}
	if target == "" {
		return "", fmt.Errorf("usage: lex %s", usage)
	}
	return target, nil
}

func resolveTargets(target string) (string, conf.Config, []string, error) {
	lexRoot, err := root.Resolve()
	if err != nil {
		return "", conf.Config{}, nil, err
	}
	cfg, err := loadConf(lexRoot)
	if err != nil {
		return "", conf.Config{}, nil, err
	}
	names := []string{target}
	if strings.HasPrefix(target, "@") {
		names, err = repo.ExpandGroup(lexRoot, cfg.Repos, target)
		if err != nil {
			return "", conf.Config{}, nil, err
		}
		if len(names) == 0 {
			return "", conf.Config{}, nil, fmt.Errorf("group %q is empty", target)
		}
	}
	return lexRoot, cfg, names, nil
}

func cmdRemove(args []string) error {
	target, err := singleTarget(args, "remove <pkg|@group>")
	if err != nil {
		return err
	}
	lexRoot, cfg, names, err := resolveTargets(target)
	if err != nil {
		return err
	}
	for _, n := range names {
		if err := remove.Remove(lexRoot, cfg, n); err != nil {
			return fmt.Errorf("%s: %w", n, err)
		}
	}
	return nil
}

func cmdVerify(args []string) error {
	target, err := singleTarget(args, "verify <pkg|@group>")
	if err != nil {
		return err
	}
	lexRoot, _, names, err := resolveTargets(target)
	if err != nil {
		return err
	}
	for _, n := range names {
		if err := remove.Verify(lexRoot, n); err != nil {
			return fmt.Errorf("%s: %w", n, err)
		}
	}
	return nil
}

func cmdSync() error {
	lexRoot, err := root.Resolve()
	if err != nil {
		return err
	}
	cfg, err := loadConf(lexRoot)
	if err != nil {
		return err
	}
	if len(cfg.Repos) == 0 {
		return fmt.Errorf("no repos configured in lex.conf")
	}
	return repo.SyncAll(lexRoot, cfg.Repos)
}

func cmdList() error {
	lexRoot, err := root.Resolve()
	if err != nil {
		return err
	}
	names, err := db.List(lexRoot)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Println("no packages installed")
		return nil
	}
	sort.Strings(names)
	for _, n := range names {
		if ins, err := db.Load(lexRoot, n); err == nil {
			fmt.Printf("%s %s (%s)\n", ins.Name, ins.Version, ins.Method)
		} else {
			fmt.Println(n)
		}
	}
	return nil
}
