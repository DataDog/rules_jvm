package maven

import (
	"errors"
	"fmt"

	"github.com/bazel-contrib/rules_jvm/java/gazelle/private/bazel"
	"github.com/bazel-contrib/rules_jvm/java/gazelle/private/maven/multiset"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/rs/zerolog"
)

type Resolver interface {
	Resolve(pkg string) (label.Label, error)
}

// resolver finds Maven provided packages by reading the maven_install.json
// file from rules_jvm_external.
type resolver struct {
	data   *multiset.StringMultiSet
	logger zerolog.Logger
}

func NewResolver(installFile string, excludedArtifacts map[string]struct{}, logger zerolog.Logger) (Resolver, error) {
	r := resolver{
		data:   multiset.NewStringMultiSet(),
		logger: logger.With().Str("_c", "maven-resolver").Logger(),
	}

	c, err := loadConfiguration(installFile)
	if err != nil {
		r.logger.Warn().Err(err).Msg("not loading maven dependencies")
		return &r, nil
	}

	r.logger.Debug().Int("count", len(c.DependencyTree.Dependencies)).Msg("Dependency count")
	r.logger.Debug().Str("conflicts", fmt.Sprintf("%#v", c.DependencyTree.ConflictResolution)).Msg("Maven install conflict")

	for _, dep := range c.DependencyTree.Dependencies {
		for _, pkg := range dep.Packages {
			c, err := ParseCoordinate(dep.Coord)
			if err != nil {
				return nil, fmt.Errorf("failed to parse coordinate %v: %w", dep.Coord, err)
			}
			l := label.New("maven", "", bazel.CleanupLabel(c.ArtifactString()))
			if _, found := excludedArtifacts[l.String()]; !found {
				r.data.Add(pkg, l.String())
			}
		}
	}

	return &r, nil
}

func (r *resolver) Resolve(pkg string) (label.Label, error) {
	v, found := r.data.Get(pkg)
	if !found {
		return label.NoLabel, fmt.Errorf("package not found: %s", pkg)
	}

	switch len(v) {
	case 0:
		return label.NoLabel, errors.New("no external imports")

	case 1:
		var ret string
		for r := range v {
			ret = r
			break
		}
		return label.Parse(ret)

	default:
		r.logger.Error().Msg("Append one of the following to BUILD.bazel:")
		for k := range v {
			r.logger.Error().Msgf("# gazelle:resolve java %s %s", pkg, k)
		}

		return label.NoLabel, errors.New("many possible imports")
	}
}

func LabelFromArtifact(artifact string) string {
	return label.New("maven", "", bazel.CleanupLabel(artifact)).String()
}
