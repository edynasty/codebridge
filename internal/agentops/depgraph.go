package agentops

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxGraphNodes = 200
	maxGraphEdges = 1000
	maxManifests  = 50
)

type DepNode struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Type    string `json:"type"`
	Version string `json:"version,omitempty"`
}

type DepEdge struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Scope   string `json:"scope,omitempty"`
	Version string `json:"version,omitempty"`
}

type DepGraphResult struct {
	Type      string    `json:"type"`
	Nodes     []DepNode `json:"nodes"`
	Edges     []DepEdge `json:"edges"`
	Truncated bool      `json:"truncated"`
}

// dependencyGraph builds a bounded module/dependency graph from the workspace's
// build manifests. Maven is preferred when present, then npm, then Go.
func (s *Service) dependencyGraph(ctx context.Context, root string) (DepGraphResult, error) {
	rootReal, err := s.resolveWorkspaceRoot(root)
	if err != nil {
		return DepGraphResult{}, err
	}
	if fileExists(filepath.Join(rootReal, "pom.xml")) {
		return s.mavenGraph(ctx, rootReal)
	}
	if fileExists(filepath.Join(rootReal, "package.json")) {
		return s.npmGraph(rootReal)
	}
	if fileExists(filepath.Join(rootReal, "go.mod")) {
		return s.goGraph(rootReal)
	}
	return DepGraphResult{}, errors.New("no supported dependency manifest found (pom.xml, package.json or go.mod)")
}

func (s *Service) mavenGraph(ctx context.Context, rootReal string) (DepGraphResult, error) {
	result := DepGraphResult{Type: "maven", Nodes: []DepNode{}, Edges: []DepEdge{}}
	poms := s.findManifests(ctx, rootReal, "pom.xml")
	if len(poms) == 0 {
		return result, errors.New("no readable pom.xml found")
	}
	truncated := false
	addNode := func(n DepNode) {
		if len(result.Nodes) >= maxGraphNodes {
			truncated = true
			return
		}
		result.Nodes = append(result.Nodes, n)
	}
	addEdge := func(e DepEdge) {
		if len(result.Edges) >= maxGraphEdges {
			truncated = true
			return
		}
		result.Edges = append(result.Edges, e)
	}

	for _, pomRel := range poms {
		select {
		case <-ctx.Done():
			result.Truncated = truncated
			return result, ctx.Err()
		default:
		}
		abs := filepath.Join(rootReal, filepath.FromSlash(pomRel))
		project, err := parseMavenPom(abs)
		if err != nil {
			continue
		}
		moduleID := project.groupID + ":" + project.artifactID
		if project.groupID == "" && project.parent.GroupID != "" {
			moduleID = project.parent.GroupID + ":" + project.artifactID
		}
		addNode(DepNode{ID: moduleID, Path: pomRel, Type: "module", Version: project.version})
		if project.parent.GroupID != "" && project.parent.ArtifactID != "" {
			parentID := project.parent.GroupID + ":" + project.parent.ArtifactID
			addEdge(DepEdge{From: moduleID, To: parentID, Scope: "parent", Version: project.parent.Version})
		}
		for _, dep := range project.dependencies {
			depID := dep.GroupID + ":" + dep.ArtifactID
			addEdge(DepEdge{From: moduleID, To: depID, Scope: dep.Scope, Version: dep.Version})
		}
	}
	result.Truncated = truncated
	sortNodesAndEdges(result)
	return result, nil
}

type mavenProject struct {
	groupID      string
	artifactID   string
	version      string
	parent       mavenCoordinate
	dependencies []mavenCoordinate
}

type mavenCoordinate struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
}

type mavenPomXML struct {
	GroupID      string          `xml:"groupId"`
	ArtifactID   string          `xml:"artifactId"`
	Version      string          `xml:"version"`
	Parent       mavenCoordinate `xml:"parent"`
	Dependencies struct {
		Dependency []mavenCoordinate `xml:"dependency"`
	} `xml:"dependencies"`
}

func parseMavenPom(abs string) (mavenProject, error) {
	b, err := os.ReadFile(abs)
	if err != nil {
		return mavenProject{}, err
	}
	var pom mavenPomXML
	if err := xml.Unmarshal(b, &pom); err != nil {
		return mavenProject{}, fmt.Errorf("parse pom.xml: %w", err)
	}
	deps := make([]mavenCoordinate, 0, len(pom.Dependencies.Dependency))
	for _, dep := range pom.Dependencies.Dependency {
		if dep.GroupID == "" || dep.ArtifactID == "" {
			continue
		}
		if dep.Scope == "" {
			dep.Scope = "compile"
		}
		deps = append(deps, dep)
	}
	return mavenProject{
		groupID:      pom.GroupID,
		artifactID:   pom.ArtifactID,
		version:      pom.Version,
		parent:       pom.Parent,
		dependencies: deps,
	}, nil
}

func (s *Service) npmGraph(rootReal string) (DepGraphResult, error) {
	result := DepGraphResult{Type: "npm", Nodes: []DepNode{}, Edges: []DepEdge{}}
	ctx := context.Background()
	manifests := s.findManifests(ctx, rootReal, "package.json")
	if len(manifests) == 0 {
		return result, errors.New("no readable package.json found")
	}
	truncated := false
	for _, rel := range manifests {
		var pkg struct {
			Name            string            `json:"name"`
			Version         string            `json:"version"`
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		b, err := os.ReadFile(filepath.Join(rootReal, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		if err := json.Unmarshal(b, &pkg); err != nil {
			continue
		}
		if pkg.Name == "" {
			continue
		}
		pkgID := "npm:" + pkg.Name
		if len(result.Nodes) < maxGraphNodes {
			result.Nodes = append(result.Nodes, DepNode{ID: pkgID, Path: rel, Type: "module", Version: pkg.Version})
		} else {
			truncated = true
		}
		appendDeps := func(deps map[string]string, scope string) {
			names := make([]string, 0, len(deps))
			for name := range deps {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				if len(result.Edges) >= maxGraphEdges {
					truncated = true
					return
				}
				result.Edges = append(result.Edges, DepEdge{From: pkgID, To: "npm:" + name, Scope: scope, Version: deps[name]})
			}
		}
		appendDeps(pkg.Dependencies, "runtime")
		appendDeps(pkg.DevDependencies, "dev")
	}
	result.Truncated = truncated
	sortNodesAndEdges(result)
	return result, nil
}

func (s *Service) goGraph(rootReal string) (DepGraphResult, error) {
	result := DepGraphResult{Type: "go", Nodes: []DepNode{}, Edges: []DepEdge{}}
	module, requires, err := parseGoMod(filepath.Join(rootReal, "go.mod"))
	if err != nil {
		return result, fmt.Errorf("parse go.mod: %w", err)
	}
	result.Nodes = append(result.Nodes, DepNode{ID: module, Path: "go.mod", Type: "module"})
	truncated := false
	for _, req := range requires {
		if len(result.Edges) >= maxGraphEdges {
			truncated = true
			break
		}
		result.Edges = append(result.Edges, DepEdge{From: module, To: req.path, Scope: "require", Version: req.version})
	}
	result.Truncated = truncated
	sortNodesAndEdges(result)
	return result, nil
}

type goRequire struct {
	path    string
	version string
}

func parseGoMod(abs string) (string, []goRequire, error) {
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", nil, err
	}
	var module string
	var requires []goRequire
	inBlock := false
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "//") || line == "" {
			continue
		}
		if inBlock {
			if line == ")" {
				inBlock = false
				continue
			}
			fields := strings.Fields(strings.TrimSuffix(line, "// indirect"))
			if len(fields) >= 2 {
				requires = append(requires, goRequire{path: fields[0], version: fields[1]})
			}
			continue
		}
		if strings.HasPrefix(line, "module ") {
			module = strings.TrimSpace(strings.TrimPrefix(line, "module "))
		} else if strings.HasPrefix(line, "require (") {
			inBlock = true
		} else if strings.HasPrefix(line, "require ") {
			fields := strings.Fields(strings.TrimPrefix(line, "require "))
			if len(fields) >= 2 {
				requires = append(requires, goRequire{path: fields[0], version: fields[1]})
			}
		}
	}
	if module == "" {
		return "", nil, errors.New("go.mod has no module directive")
	}
	return module, requires, nil
}

// findManifests walks the workspace for one manifest file name, bounded in
// depth and count, applying the shared skip/sensitive policy.
func (s *Service) findManifests(ctx context.Context, rootReal, name string) []string {
	var out []string
	_ = filepath.WalkDir(rootReal, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fs.SkipAll
		default:
		}
		rel, relErr := filepath.Rel(rootReal, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if path != rootReal && (shouldSkipDir(d.Name()) || (!s.AllowSensitiveFiles && isSensitivePath(rel))) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != name || (!s.AllowSensitiveFiles && isSensitivePath(rel)) {
			return nil
		}
		out = append(out, rel)
		if len(out) >= maxManifests {
			return fs.SkipAll
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func sortNodesAndEdges(result DepGraphResult) {
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].ID < result.Nodes[j].ID })
	sort.Slice(result.Edges, func(i, j int) bool {
		if result.Edges[i].From != result.Edges[j].From {
			return result.Edges[i].From < result.Edges[j].From
		}
		return result.Edges[i].To < result.Edges[j].To
	})
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
