package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds all settings, sourced from .grafux.yml and overridden by CLI flags.
type Config struct {
	Depth       int      `yaml:"depth"`
	Port        int      `yaml:"port"`
	NoOpen      bool     `yaml:"no-open"`
	ShowHidden  bool     `yaml:"show-hidden"`
	Theme       string   `yaml:"theme"`
	FileRadius  float64  `yaml:"file-radius"`  // base radius for file nodes
	FolderBase  float64  `yaml:"folder-base"`  // base radius for folder nodes
	FolderScale float64  `yaml:"folder-scale"` // how much folder radius grows with children
	EdgeWidth   float64  `yaml:"edge-width"`   // edge line width multiplier
	LabelZoom   float64  `yaml:"label-zoom"`   // zoom level at which all labels appear
	Include     []string `yaml:"include"`       // only show files with these extensions, e.g. [".go",".md"]
	Exclude     []string `yaml:"exclude"`       // hide files with these extensions
	// Physics
	ChargeStrength  float64 `yaml:"charge-strength"`
	ChargeMax       float64 `yaml:"charge-max"`
	LinkDistance     float64 `yaml:"link-distance"`
	LinkStrength     float64 `yaml:"link-strength"`
	CenterStrength  float64 `yaml:"center-strength"`
	CollideStrength float64 `yaml:"collide-strength"`
	AlphaDecay      float64 `yaml:"alpha-decay"`
	VelocityDecay   float64 `yaml:"velocity-decay"`
	// Layout
	Layout string `yaml:"layout"` // "force", "radial", or "tree"
}

func Defaults() Config {
	return Config{
		Depth:           5,
		Theme:           "gruvbox",
		FileRadius:      5,
		FolderBase:      8,
		FolderScale:     2.5,
		EdgeWidth:       1.0,
		LabelZoom:       2.0,
		ChargeStrength:  -100,
		ChargeMax:       400,
		LinkDistance:     55,
		LinkStrength:    0.4,
		CenterStrength:  0.05,
		CollideStrength: 0.8,
		AlphaDecay:      0.02,
		VelocityDecay:   0.35,
		Layout:          "force",
	}
}

// Load reads .grafux.yml from dir. A missing file is not an error.
func Load(dir string) (Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(filepath.Join(dir, ".grafux.yml"))
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	return cfg, yaml.Unmarshal(data, &cfg)
}
