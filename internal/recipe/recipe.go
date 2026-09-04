package recipe

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type BinaryStanza struct {
	URL        string   `json:"url"`
	SHA256     string   `json:"sha256"`
	Install    []string `json:"install"`
	PostRemove []string `json:"post_remove,omitempty"`
}

type SourceStanza struct {
	URL        string   `json:"url"`
	SHA256     string   `json:"sha256"`
	Patches    []string `json:"patches,omitempty"`
	Build      []string `json:"build,omitempty"`
	Install    []string `json:"install"`
	PostRemove []string `json:"post_remove,omitempty"`
}

type Recipe struct {
	Kind        string        `json:"kind"`
	Name        string        `json:"name"`
	Version     string        `json:"version"`
	Arch        []string      `json:"arch,omitempty"`
	Deps        []string      `json:"deps,omitempty"`
	Homepage    string        `json:"homepage,omitempty"`
	Description string        `json:"description,omitempty"`
	Binary      *BinaryStanza `json:"binary,omitempty"`
	Source      *SourceStanza `json:"source,omitempty"`
	// non-file side effects only
	PostRemove []string `json:"post_remove,omitempty"`
}

func Load(path string) (*Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Recipe
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("invalid recipe %s: %w", path, err)
	}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("invalid recipe %s: %w", path, err)
	}
	return &r, nil
}

func (r *Recipe) Validate() error {
	if r.Kind != "" && r.Kind != "recipe" {
		return fmt.Errorf("kind must be %q", "recipe")
	}
	if !nameRe.MatchString(r.Name) {
		return fmt.Errorf("invalid name %q", r.Name)
	}
	if r.Version == "" {
		return fmt.Errorf("missing version")
	}
	if r.Binary == nil && r.Source == nil {
		return fmt.Errorf("need at least one of binary|source")
	}
	if r.Binary != nil && (r.Binary.URL == "" || r.Binary.SHA256 == "") {
		return fmt.Errorf("binary needs url+sha256")
	}
	if r.Source != nil && (r.Source.URL == "" || r.Source.SHA256 == "") {
		return fmt.Errorf("source needs url+sha256")
	}
	return nil
}

func NormalizeArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	default:
		return goarch
	}
}

// empty Arch or "any" matches everything; amd64 and x86_64 match
func (r *Recipe) SupportsArch(goarch string) bool {
	if len(r.Arch) == 0 {
		return true
	}
	want := NormalizeArch(goarch)
	for _, a := range r.Arch {
		if a == "any" || NormalizeArch(a) == want {
			return true
		}
	}
	return false
}

// falls back when prefer is missing
func (r *Recipe) SelectMethod(prefer string) (string, error) {
	switch prefer {
	case "binary":
		if r.Binary != nil {
			return "binary", nil
		}
		if r.Source != nil {
			return "source", nil
		}
	case "source":
		if r.Source != nil {
			return "source", nil
		}
		if r.Binary != nil {
			return "binary", nil
		}
	}
	return "", fmt.Errorf("no installable stanza (prefer=%s)", prefer)
}
