package files

import (
	"fmt"
	"sort"
	"strings"
)

// TreeEntry is one visible file-tree row. Detail is the file's git status or the highest-priority status below a directory.
type TreeEntry struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Directory bool   `json:"directory"`
	Depth     int    `json:"depth"`
	Expanded  bool   `json:"expanded"`
	Selected  bool   `json:"selected"`
	Detail    string `json:"detail,omitempty"`
}

// TreeSnapshot is the current value of one tree route. A null selected path means that no row is selected.
type TreeSnapshot struct {
	Generation uint64      `json:"generation"`
	Entries    []TreeEntry `json:"entries"`
	Selected   *string     `json:"selected"`
}

type treeRoute struct {
	root       string
	generation uint64
	expanded   map[string]bool
	selected   *string
	listings   map[string][]Entry
}

type treeLister func(root, relative string) ([]Entry, error)
type treeWatchSet func(surface, root string, paths []string, reason string, trace Trace) error

// treeRoutes is owned by one Serve process, so its surface key is scoped to one window as well as one sidecar session.
type treeRoutes struct {
	current    map[string]*treeRoute
	generation map[string]uint64
}

func newTreeRoutes() *treeRoutes {
	return &treeRoutes{current: map[string]*treeRoute{}, generation: map[string]uint64{}}
}

func (routes *treeRoutes) snapshot(surface, root string) *treeRoute {
	if current := routes.current[surface]; current != nil && current.root == root {
		return current
	}
	routes.generation[surface]++
	if routes.generation[surface] == 0 {
		routes.generation[surface] = 1
	}
	current := &treeRoute{root: root, generation: routes.generation[surface], expanded: map[string]bool{"": true}, listings: map[string][]Entry{}}
	routes.current[surface] = current
	return current
}

func (routes *treeRoutes) close(surface string) {
	delete(routes.current, surface)
	delete(routes.generation, surface)
}

func (routes *treeRoutes) route(surface, root string, generation uint64) (*treeRoute, error) {
	current := routes.current[surface]
	if current == nil {
		return nil, fmt.Errorf("tree snapshot is required before this operation")
	}
	if current.root != root {
		return nil, fmt.Errorf("tree route root changed; request a snapshot")
	}
	if generation != current.generation {
		return nil, fmt.Errorf("stale tree generation %d; current generation is %d", generation, current.generation)
	}
	return current, nil
}

var gitDetailPriority = map[string]int{"deleted": 5, "modified": 4, "added": 3, "renamed": 2, "untracked": 1}

func (route *treeRoute) update(root, surface, operation string, request Request, trace Trace, list treeLister, setWatches treeWatchSet) (*TreeSnapshot, error) {
	switch operation {
	case "snapshot":
	case "expand", "collapse", "select":
		if request.Body.Generation != route.generation {
			return nil, fmt.Errorf("stale tree generation %d; current generation is %d", request.Body.Generation, route.generation)
		}
	default:
		return nil, fmt.Errorf("unknown tree operation %q", operation)
	}

	if operation == "expand" {
		if request.Body.Path == "" {
			return nil, fmt.Errorf("cannot expand the project root")
		}
		entry, ok := route.entry(request.Body.Path)
		if !ok || !entry.Directory {
			return nil, fmt.Errorf("%s is not a listed directory", request.Body.Path)
		}
		route.expanded[request.Body.Path] = true
	} else if operation == "collapse" {
		if request.Body.Path == "" {
			return nil, fmt.Errorf("cannot collapse the project root")
		}
		entry, ok := route.entry(request.Body.Path)
		if !ok || !entry.Directory {
			return nil, fmt.Errorf("%s is not a listed directory", request.Body.Path)
		}
		for path := range route.expanded {
			if path == request.Body.Path || strings.HasPrefix(path, request.Body.Path+"/") {
				delete(route.expanded, path)
			}
		}
		if route.selected != nil && (*route.selected == request.Body.Path || strings.HasPrefix(*route.selected, request.Body.Path+"/")) {
			route.selected = nil
		}
	} else if operation == "select" {
		if request.Body.Path == "" {
			route.selected = nil
		} else {
			if _, ok := route.entry(request.Body.Path); !ok {
				return nil, fmt.Errorf("%s is not a listed path", request.Body.Path)
			}
			selected := request.Body.Path
			route.selected = &selected
		}
	}

	fresh := map[string][]Entry{}
	paths := make([]string, 0, len(route.expanded))
	for path := range route.expanded {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		depthI, depthJ := strings.Count(paths[i], "/"), strings.Count(paths[j], "/")
		if paths[i] == "" {
			depthI = -1
		}
		if paths[j] == "" {
			depthJ = -1
		}
		if depthI != depthJ {
			return depthI < depthJ
		}
		return paths[i] < paths[j]
	})
	for _, path := range paths {
		if path != "" {
			parent := ""
			if at := strings.LastIndex(path, "/"); at >= 0 {
				parent = path[:at]
			}
			if _, ok := fresh[parent]; !ok || !containsDirectory(fresh[parent], baseName(path)) {
				delete(route.expanded, path)
				continue
			}
		}
		entries, err := list(root, path)
		if err != nil {
			if path == "" {
				return nil, err
			}
			return nil, fmt.Errorf("cannot list expanded directory %s: %w", path, err)
		}
		fresh[path] = entries
	}
	route.listings = fresh
	if len(route.expanded) == 0 {
		route.expanded[""] = true
	}
	watchPaths := make([]string, 0, len(route.expanded))
	for path := range route.expanded {
		watchPaths = append(watchPaths, path)
	}
	sort.Strings(watchPaths)
	if err := setWatches(surface, root, watchPaths, "replaced", trace); err != nil {
		return nil, err
	}

	git, err := GitStatus(root, trace)
	if err != nil {
		return nil, err
	}
	gitByPath := make(map[string]string, len(git))
	for _, change := range git {
		gitByPath[change.Path] = change.Status
	}
	rows := make([]TreeEntry, 0)
	var visit func(string, int)
	visit = func(path string, depth int) {
		for _, entry := range route.listings[path] {
			child := entry.Name
			if path != "" {
				child = path + "/" + entry.Name
			}
			row := TreeEntry{Path: child, Name: entry.Name, Directory: entry.Directory, Depth: depth, Expanded: entry.Directory && route.expanded[child]}
			if route.selected != nil && *route.selected == child {
				row.Selected = true
			}
			if entry.Directory {
				for changed, status := range gitByPath {
					if strings.HasPrefix(changed, child+"/") && gitDetailPriority[status] > gitDetailPriority[row.Detail] {
						row.Detail = status
					}
				}
			} else {
				row.Detail = gitByPath[child]
			}
			rows = append(rows, row)
			if row.Expanded {
				visit(child, depth+1)
			}
		}
	}
	visit("", 0)
	if route.selected != nil {
		visible := false
		for _, row := range rows {
			if row.Path == *route.selected {
				visible = true
				break
			}
		}
		if !visible {
			route.selected = nil
		}
	}
	return &TreeSnapshot{Generation: route.generation, Entries: rows, Selected: route.selected}, nil
}

func (route *treeRoute) entry(path string) (Entry, bool) {
	parent := ""
	name := path
	if at := strings.LastIndex(path, "/"); at >= 0 {
		parent, name = path[:at], path[at+1:]
	}
	for _, entry := range route.listings[parent] {
		if entry.Name == name {
			return entry, true
		}
	}
	return Entry{}, false
}

func containsDirectory(entries []Entry, name string) bool {
	for _, entry := range entries {
		if entry.Name == name && entry.Directory {
			return true
		}
	}
	return false
}

func baseName(path string) string {
	if at := strings.LastIndex(path, "/"); at >= 0 {
		return path[at+1:]
	}
	return path
}
