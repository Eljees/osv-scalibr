// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gem_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/osv-scalibr/extractor"
	"github.com/google/osv-scalibr/extractor/filesystem"
	"github.com/google/osv-scalibr/extractor/filesystem/internal/units"
	"github.com/google/osv-scalibr/extractor/filesystem/language/ruby/gem"
	"github.com/google/osv-scalibr/extractor/filesystem/simplefileapi"
	scalibrfs "github.com/google/osv-scalibr/fs"
	"github.com/google/osv-scalibr/inventory"
	"github.com/google/osv-scalibr/purl"
	"github.com/google/osv-scalibr/stats"
	"github.com/google/osv-scalibr/testing/extracttest"
	"github.com/google/osv-scalibr/testing/fakefs"
	"github.com/google/osv-scalibr/testing/testcollector"
	"gopkg.in/yaml.v3"

	cpb "github.com/google/osv-scalibr/binary/proto/config_go_proto"
)

func TestFileRequired(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		fileSizeBytes    int64
		maxFileSizeBytes int64
		wantRequired     bool
		wantResultMetric stats.FileRequiredResult
	}{
		{
			name:             ".gem at root",
			path:             "rails.gem",
			wantRequired:     true,
			wantResultMetric: stats.FileRequiredResultOK,
		},
		{
			name:             ".gem nested path",
			path:             "testdata/aws-sdk-core-3.218.0.gem",
			wantRequired:     true,
			wantResultMetric: stats.FileRequiredResultOK,
		},
		{
			name:         "metadata.gz at root",
			path:         "metadata.gz",
			wantRequired: false,
		},
		{
			name:         "metadata.gz nested path",
			path:         "testdata/metadata.gz",
			wantRequired: false,
		},
		{
			name:         "not a gem or metadata file",
			path:         "testdata/test.rb",
			wantRequired: false,
		},
		{
			name:         "data.tar.gz inside gem archive",
			path:         "data.tar.gz",
			wantRequired: false,
		},
		{
			name:             ".gem required if size less than maxFileSizeBytes",
			path:             "test.gem",
			fileSizeBytes:    10 * units.MiB,
			maxFileSizeBytes: 20 * units.MiB,
			wantRequired:     true,
			wantResultMetric: stats.FileRequiredResultOK,
		},
		{
			name:             ".gem required if size equal to maxFileSizeBytes",
			path:             "test.gem",
			fileSizeBytes:    10 * units.MiB,
			maxFileSizeBytes: 10 * units.MiB,
			wantRequired:     true,
			wantResultMetric: stats.FileRequiredResultOK,
		},
		{
			name:             ".gem not required if size greater than maxFileSizeBytes",
			path:             "test.gem",
			fileSizeBytes:    50 * units.MiB,
			maxFileSizeBytes: 10 * units.MiB,
			wantRequired:     false,
			wantResultMetric: stats.FileRequiredResultSizeLimitExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := testcollector.New()
			e, err := gem.New(&cpb.PluginConfig{MaxFileSizeBytes: tt.maxFileSizeBytes})
			if err != nil {
				t.Fatalf("gem.New: %v", err)
			}
			e.(*gem.Extractor).Stats = collector

			fileSizeBytes := tt.fileSizeBytes
			if fileSizeBytes == 0 {
				fileSizeBytes = 1 * units.KiB
			}

			isRequired := e.FileRequired(simplefileapi.New(tt.path, fakefs.FakeFileInfo{
				FileName: filepath.Base(tt.path),
				FileMode: fs.ModePerm,
				FileSize: fileSizeBytes,
			}))
			if isRequired != tt.wantRequired {
				t.Fatalf("FileRequired(%s): got %v, want %v", tt.path, isRequired, tt.wantRequired)
			}

			gotResultMetric := collector.FileRequiredResult(tt.path)
			if gotResultMetric != tt.wantResultMetric {
				t.Errorf("FileRequired(%s) recorded result metric %v, want result metric %v", tt.path, gotResultMetric, tt.wantResultMetric)
			}
		})
	}
}

func TestExtract(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		includeDeps      bool
		wantPackages     []*extractor.Package
		wantErr          error
		wantResultMetric stats.FileExtractedResult
	}{
		{
			name:        "aws-sdk-core without dependencies",
			path:        "testdata/aws-sdk-core-3.218.0.gem",
			includeDeps: false,
			wantPackages: []*extractor.Package{
				{
					Name:     "aws-sdk-core",
					Version:  "3.218.0",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/aws-sdk-core-3.218.0.gem"),
					Metadata: &gem.RubyGemMetadata{
						Authors:     []string{"Amazon Web Services"},
						Description: "Provides API clients for AWS. This gem is part of the official AWS SDK for Ruby.",
						Homepage:    "https://github.com/aws/aws-sdk-ruby",
						Licenses:    []string{"Apache-2.0"},
						Platform:    "ruby",
						Summary:     "AWS SDK for Ruby - Core",
					},
				},
			},
		},
		{
			name:        "aws-sdk-core with dependencies",
			path:        "testdata/aws-sdk-core-3.218.0.gem",
			includeDeps: true,
			wantPackages: []*extractor.Package{
				{
					Name:     "aws-sdk-core",
					Version:  "3.218.0",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/aws-sdk-core-3.218.0.gem"),
					Metadata: &gem.RubyGemMetadata{
						Authors:     []string{"Amazon Web Services"},
						Description: "Provides API clients for AWS. This gem is part of the official AWS SDK for Ruby.",
						Homepage:    "https://github.com/aws/aws-sdk-ruby",
						Licenses:    []string{"Apache-2.0"},
						Platform:    "ruby",
						Summary:     "AWS SDK for Ruby - Core",
					},
				},
				{
					Name:     "jmespath",
					Version:  "1.6.1",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/aws-sdk-core-3.218.0.gem"),
				},
				{
					Name:     "aws-partitions",
					Version:  "1.992.0",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/aws-sdk-core-3.218.0.gem"),
				},
				{
					Name:     "aws-sigv4",
					Version:  "1.9.0",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/aws-sdk-core-3.218.0.gem"),
				},
				{
					Name:     "aws-eventstream",
					Version:  "1.3.0",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/aws-sdk-core-3.218.0.gem"),
				},
			},
		},
		{
			name:        "faraday without dependencies",
			path:        "testdata/faraday-2.12.2.gem",
			includeDeps: false,
			wantPackages: []*extractor.Package{
				{
					Name:     "faraday",
					Version:  "2.12.2",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/faraday-2.12.2.gem"),
					Metadata: &gem.RubyGemMetadata{
						Authors:     []string{"@technoweenie", "@iMacTia", "@olleolleolle"},
						Description: "",
						Homepage:    "https://lostisland.github.io/faraday",
						Licenses:    []string{"MIT"},
						Platform:    "ruby",
						Summary:     "HTTP/REST API client library.",
					},
				},
			},
		},
		{
			name:        "faraday with dependencies",
			path:        "testdata/faraday-2.12.2.gem",
			includeDeps: true,
			wantPackages: []*extractor.Package{
				{
					Name:     "faraday",
					Version:  "2.12.2",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/faraday-2.12.2.gem"),
					Metadata: &gem.RubyGemMetadata{
						Authors:     []string{"@technoweenie", "@iMacTia", "@olleolleolle"},
						Description: "",
						Homepage:    "https://lostisland.github.io/faraday",
						Licenses:    []string{"MIT"},
						Platform:    "ruby",
						Summary:     "HTTP/REST API client library.",
					},
				},
				{
					Name:     "faraday-net_http",
					Version:  "2.0",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/faraday-2.12.2.gem"),
				},
				{
					Name:     "json",
					Version:  "0",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/faraday-2.12.2.gem"),
				},
				{
					Name:     "logger",
					Version:  "0",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/faraday-2.12.2.gem"),
				},
			},
		},
		{
			name:        "rack with only dev dependencies returns no runtime dependencies",
			path:        "testdata/rack-3.1.8.gem",
			includeDeps: true,
			wantPackages: []*extractor.Package{
				{
					Name:     "rack",
					Version:  "3.1.8",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/rack-3.1.8.gem"),
					Metadata: &gem.RubyGemMetadata{
						Authors: []string{"Leah Neukirchen"},
						Description: "Rack provides a minimal, modular and adaptable interface for developing\n" +
							"web applications in Ruby. By wrapping HTTP requests and responses in\n" +
							"the simplest way possible, it unifies and distills the API for web\n" +
							"servers, web frameworks, and software in between (the so-called\n" +
							"middleware) into a single method call.\n",
						Homepage: "https://github.com/rack/rack",
						Licenses: []string{"MIT"},
						Platform: "ruby",
						Summary:  "A modular Ruby webserver interface.",
					},
				},
			},
		},
		{
			name:        "synthetic gem with exact versions, twiddle-wakka normalization, and skips",
			path:        "testdata/synthetic_exact-1.0.0.gem",
			includeDeps: true,
			wantPackages: []*extractor.Package{
				{
					Name:     "synthetic-gem",
					Version:  "1.0.0",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/synthetic_exact-1.0.0.gem"),
					Metadata: &gem.RubyGemMetadata{
						Authors:     []string{"Test Author"},
						Description: "Synthetic test gem.",
						Homepage:    "https://example.com/synthetic",
						Licenses:    []string{"MIT"},
						Platform:    "ruby",
						Summary:     "Synthetic test gem.",
					},
				},
				{
					Name:     "exact-dep",
					Version:  "2.0.0",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/synthetic_exact-1.0.0.gem"),
				},
				{
					Name:     "twiddle-single",
					Version:  "1.0.0",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/synthetic_exact-1.0.0.gem"),
				},
				{
					Name:     "twiddle-double",
					Version:  "2.3.0",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/synthetic_exact-1.0.0.gem"),
				},
				{
					Name:     "twiddle-triple",
					Version:  "4.5.6",
					PURLType: purl.TypeGem,
					Location: extractor.LocationFromPath("testdata/synthetic_exact-1.0.0.gem"),
				},
			},
		},
		{
			name:             "corrupt gem archive",
			path:             "testdata/corrupt.gem",
			wantErr:          cmpopts.AnyError,
			wantResultMetric: stats.FileExtractedResultErrorUnknown,
		},
		{
			name:             "corrupt metadata inside gem",
			path:             "testdata/corrupt_metadata.gem",
			wantErr:          cmpopts.AnyError,
			wantResultMetric: stats.FileExtractedResultErrorUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := os.Open(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := r.Close(); err != nil {
					t.Errorf("Close(): %v", err)
				}
			}()

			info, err := os.Stat(tt.path)
			if err != nil {
				t.Fatal(err)
			}

			collector := testcollector.New()
			input := &filesystem.ScanInput{
				FS:     scalibrfs.DirFS("."),
				Path:   tt.path,
				Reader: r,
				Info:   info,
			}
			cfg := &cpb.PluginConfig{
				PluginSpecific: []*cpb.PluginSpecificConfig{
					{Config: &cpb.PluginSpecificConfig_RubyGem{
						RubyGem: &cpb.RubyGemConfig{
							IncludeDependencies: tt.includeDeps,
						},
					}},
				},
			}
			e, err := gem.New(cfg)
			if err != nil {
				t.Fatalf("gem.New: %v", err)
			}
			e.(*gem.Extractor).Stats = collector
			got, err := e.Extract(t.Context(), input)
			if !cmp.Equal(err, tt.wantErr, cmpopts.EquateErrors()) {
				t.Fatalf("Extract(%+v) error: got %v, want %v\n", tt.name, err, tt.wantErr)
			}

			var want inventory.Inventory
			if tt.wantPackages != nil {
				want = inventory.Inventory{Packages: tt.wantPackages}
			}

			if diff := cmp.Diff(
				want,
				got,
				cmpopts.SortSlices(extracttest.PackageCmpLess),
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreFields(gem.RubyGemMetadata{}, "Dependencies"),
			); diff != "" {
				t.Errorf("Extract(%s) (-want +got):\n%s", tt.path, diff)
			}

			wantResultMetric := tt.wantResultMetric
			if wantResultMetric == "" && tt.wantErr == nil {
				wantResultMetric = stats.FileExtractedResultSuccess
			}
			gotResultMetric := collector.FileExtractedResult(tt.path)
			if gotResultMetric != wantResultMetric {
				t.Errorf("Extract(%s) recorded result metric %v, want result metric %v", tt.path, gotResultMetric, wantResultMetric)
			}

			gotFileSizeMetric := collector.FileExtractedFileSize(tt.path)
			if gotFileSizeMetric != info.Size() {
				t.Errorf("Extract(%s) recorded file size %v, want file size %v", tt.path, gotFileSizeMetric, info.Size())
			}
		})
	}
}

func TestNormalizeTwiddleWakka(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1", "1.0.0"},
		{"2.3", "2.3.0"},
		{"4.5.6", "4.5.6"},
		{"1.2.3.4", "1.2.3.4"},
		{"0", "0.0.0"},
		{"0.1", "0.1.0"},
	}

	for _, tt := range tests {
		got := gem.NormalizeTwiddleWakka(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeTwiddleWakka(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveDependencyVersion(t *testing.T) {
	tests := []struct {
		name    string
		reqs    []gem.RequirementConstraint
		wantVer string
		wantOK  bool
	}{
		{
			name: "exact version",
			reqs: []gem.RequirementConstraint{
				{Operator: "=", Version: "2.1.0"},
			},
			wantVer: "2.1.0",
			wantOK:  true,
		},
		{
			name: "greater than or equal",
			reqs: []gem.RequirementConstraint{
				{Operator: ">=", Version: "1.5"},
			},
			wantVer: "1.5",
			wantOK:  true,
		},
		{
			name: "twiddle single segment",
			reqs: []gem.RequirementConstraint{
				{Operator: "~>", Version: "1"},
			},
			wantVer: "1.0.0",
			wantOK:  true,
		},
		{
			name: "twiddle two segments",
			reqs: []gem.RequirementConstraint{
				{Operator: "~>", Version: "1.9"},
			},
			wantVer: "1.9.0",
			wantOK:  true,
		},
		{
			name: "twiddle three segments",
			reqs: []gem.RequirementConstraint{
				{Operator: "~>", Version: "2.3.4"},
			},
			wantVer: "2.3.4",
			wantOK:  true,
		},
		{
			name: "multiple lower bounds picks highest",
			reqs: []gem.RequirementConstraint{
				{Operator: "~>", Version: "1"},
				{Operator: ">=", Version: "1.6.1"},
			},
			wantVer: "1.6.1",
			wantOK:  true,
		},
		{
			name: "lower and upper bound picks lower bound",
			reqs: []gem.RequirementConstraint{
				{Operator: ">=", Version: "2.0"},
				{Operator: "<", Version: "3.5"},
			},
			wantVer: "2.0",
			wantOK:  true,
		},
		{
			name: "only upper bounds returns false",
			reqs: []gem.RequirementConstraint{
				{Operator: "<", Version: "3.0.0"},
				{Operator: "<=", Version: "2.5.0"},
				{Operator: "!=", Version: "1.0.0"},
			},
			wantVer: "",
			wantOK:  false,
		},
		{
			name:    "empty requirements returns false",
			reqs:    []gem.RequirementConstraint{},
			wantVer: "",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVer, gotOK := gem.ResolveDependencyVersion(tt.reqs)
			if gotOK != tt.wantOK || gotVer != tt.wantVer {
				t.Errorf("ResolveDependencyVersion(%+v) = (%q, %v), want (%q, %v)", tt.reqs, gotVer, gotOK, tt.wantVer, tt.wantOK)
			}
		})
	}
}

func TestRubyGemMetadataDependencies(t *testing.T) {
	r, err := os.Open("testdata/synthetic_exact-1.0.0.gem")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	info, err := os.Stat("testdata/synthetic_exact-1.0.0.gem")
	if err != nil {
		t.Fatal(err)
	}

	e, err := gem.New(&cpb.PluginConfig{})
	if err != nil {
		t.Fatalf("gem.New: %v", err)
	}

	res, err := e.Extract(t.Context(), &filesystem.ScanInput{
		FS:     scalibrfs.DirFS("."),
		Path:   "testdata/synthetic_exact-1.0.0.gem",
		Reader: r,
		Info:   info,
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(res.Packages) != 1 {
		t.Fatalf("expected 1 root package, got %d", len(res.Packages))
	}

	meta, ok := res.Packages[0].Metadata.(*gem.RubyGemMetadata)
	if !ok || meta == nil {
		t.Fatalf("expected *gem.RubyGemMetadata, got %T", res.Packages[0].Metadata)
	}

	// Verify raw dependencies list in metadata
	if len(meta.Dependencies) != 6 {
		t.Fatalf("expected 6 dependencies in metadata, got %d", len(meta.Dependencies))
	}

	// Verify IsRuntime() on runtime and development dependencies
	for _, dep := range meta.Dependencies {
		if dep.Name == "dev-dep" {
			if dep.IsRuntime() {
				t.Errorf("expected dev-dep to not be runtime")
			}
		} else {
			if !dep.IsRuntime() {
				t.Errorf("expected %s to be runtime", dep.Name)
			}
		}
	}
}

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name                 string
		cfg                  *cpb.PluginConfig
		wantName             string
		wantVersion          int
		wantMaxFileSizeBytes int64
	}{
		{
			name:                 "default config",
			cfg:                  &cpb.PluginConfig{},
			wantName:             "ruby/gem",
			wantVersion:          0,
			wantMaxFileSizeBytes: 100 * units.MiB,
		},
		{
			name:                 "generic max file size",
			cfg:                  &cpb.PluginConfig{MaxFileSizeBytes: 20 * units.MiB},
			wantName:             "ruby/gem",
			wantVersion:          0,
			wantMaxFileSizeBytes: 20 * units.MiB,
		},
		{
			name: "plugin specific max file size overrides generic",
			cfg: &cpb.PluginConfig{
				MaxFileSizeBytes: 20 * units.MiB,
				PluginSpecific: []*cpb.PluginSpecificConfig{
					{Config: &cpb.PluginSpecificConfig_RubyGem{
						RubyGem: &cpb.RubyGemConfig{
							MaxFileSizeBytes: 50 * units.MiB,
						},
					}},
				},
			},
			wantName:             "ruby/gem",
			wantVersion:          0,
			wantMaxFileSizeBytes: 50 * units.MiB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := gem.New(tt.cfg)
			if err != nil {
				t.Fatalf("gem.New(%v): %v", tt.cfg, err)
			}
			if e.Name() != tt.wantName {
				t.Errorf("e.Name() = %q, want %q", e.Name(), tt.wantName)
			}
			if e.Version() != tt.wantVersion {
				t.Errorf("e.Version() = %d, want %d", e.Version(), tt.wantVersion)
			}
			if reqs := e.Requirements(); reqs == nil {
				t.Errorf("e.Requirements() is nil")
			}
		})
	}
}

func TestYAMLVersionShapes(t *testing.T) {
	// Tests unmarshaling both YAML version shapes:
	// 1. Ruby Object Mapping (!ruby/object:Gem::Version) produced by Psych
	// 2. Plain Scalar String produced by custom tools or simplified specs
	// 3. Mixed shapes within the same dependency requirement list
	tests := []struct {
		name        string
		yamlData    string
		wantDepName string
		wantReqs    []gem.RequirementConstraint
		wantVersion string
		wantOK      bool
	}{
		{
			name: "ruby object mapping version (!ruby/object:Gem::Version)",
			yamlData: `
name: rack
type: :runtime
requirement:
  requirements:
  - - "~>"
    - !ruby/object:Gem::Version
      version: '2.1.0'
  - - ">="
    - !ruby/object:Gem::Version
      version: '2.0'
`,
			wantDepName: "rack",
			wantReqs: []gem.RequirementConstraint{
				{Operator: "~>", Version: "2.1.0"},
				{Operator: ">=", Version: "2.0"},
			},
			wantVersion: "2.1.0",
			wantOK:      true,
		},
		{
			name: "plain scalar string version",
			yamlData: `
name: sinatra
type: :runtime
requirement:
  requirements:
  - - ">="
    - '1.4.0'
  - - "<"
    - '3.0.0'
`,
			wantDepName: "sinatra",
			wantReqs: []gem.RequirementConstraint{
				{Operator: ">=", Version: "1.4.0"},
				{Operator: "<", Version: "3.0.0"},
			},
			wantVersion: "1.4.0",
			wantOK:      true,
		},
		{
			name: "mixed mapping and scalar version shapes",
			yamlData: `
name: faraday
type: :runtime
requirement:
  requirements:
  - - "~>"
    - !ruby/object:Gem::Version
      version: '1.8'
  - - ">="
    - '1.8.2'
`,
			wantDepName: "faraday",
			wantReqs: []gem.RequirementConstraint{
				{Operator: "~>", Version: "1.8"},
				{Operator: ">=", Version: "1.8.2"},
			},
			wantVersion: "1.8.2",
			wantOK:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dep gem.Dependency
			if err := yaml.Unmarshal([]byte(tt.yamlData), &dep); err != nil {
				t.Fatalf("yaml.Unmarshal: %v", err)
			}
			if dep.Name != tt.wantDepName {
				t.Errorf("dep.Name = %q, want %q", dep.Name, tt.wantDepName)
			}
			if !dep.IsRuntime() {
				t.Errorf("dep.IsRuntime() = false, want true")
			}
			gotReqs := dep.Requirements()
			if diff := cmp.Diff(tt.wantReqs, gotReqs); diff != "" {
				t.Errorf("dep.Requirements() mismatch (-want +got):\n%s", diff)
			}
			gotVer, gotOK := gem.ResolveDependencyVersion(gotReqs)
			if gotOK != tt.wantOK || gotVer != tt.wantVersion {
				t.Errorf("ResolveDependencyVersion() = (%q, %v), want (%q, %v)", gotVer, gotOK, tt.wantVersion, tt.wantOK)
			}
		})
	}
}
