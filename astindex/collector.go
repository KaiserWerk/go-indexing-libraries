package astindex

import "sort"

type fileCollector struct {
	path         string
	language     Language
	contextID    string
	rootNodeID   string
	nodesByID    map[string]Node
	nodeOrder    []string
	relations    []Relation
	relationSeen map[string]struct{}
}

func newFileCollector(path string, language Language, contextID string) *fileCollector {
	return &fileCollector{
		path:         path,
		language:     language,
		contextID:    contextID,
		nodesByID:    make(map[string]Node),
		nodeOrder:    make([]string, 0, 64),
		relations:    make([]Relation, 0, 64),
		relationSeen: make(map[string]struct{}),
	}
}

func (c *fileCollector) addNode(node Node) Node {
	if existing, ok := c.nodesByID[node.ID]; ok {
		if len(existing.LanguageSpecific) == 0 && len(node.LanguageSpecific) > 0 {
			existing.LanguageSpecific = node.LanguageSpecific
			c.nodesByID[node.ID] = existing
		}
		return existing
	}
	c.nodesByID[node.ID] = node
	c.nodeOrder = append(c.nodeOrder, node.ID)
	if node.Kind == NodeKindFileRoot {
		c.rootNodeID = node.ID
	}
	return node
}

func (c *fileCollector) addRelation(rel Relation) {
	key := string(rel.Type) + "|" + rel.FromID + "|" + rel.ToID + "|" + rel.Location.Path + "|" + itoa(rel.Location.LineStart)
	if _, ok := c.relationSeen[key]; ok {
		return
	}
	c.relationSeen[key] = struct{}{}
	c.relations = append(c.relations, rel)
}

func (c *fileCollector) result() FileAST {
	nodes := make([]Node, 0, len(c.nodeOrder))
	for _, id := range c.nodeOrder {
		nodes = append(nodes, c.nodesByID[id])
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Span.LineStart == nodes[j].Span.LineStart {
			return nodes[i].ID < nodes[j].ID
		}
		return nodes[i].Span.LineStart < nodes[j].Span.LineStart
	})
	sort.Slice(c.relations, func(i, j int) bool {
		if c.relations[i].Type == c.relations[j].Type {
			if c.relations[i].FromID == c.relations[j].FromID {
				return c.relations[i].ToID < c.relations[j].ToID
			}
			return c.relations[i].FromID < c.relations[j].FromID
		}
		return c.relations[i].Type < c.relations[j].Type
	})
	return FileAST{
		Path:       c.path,
		Language:   c.language,
		ContextID:  c.contextID,
		RootNodeID: c.rootNodeID,
		Nodes:      nodes,
		Relations:  c.relations,
	}
}

func makeNode(language Language, kind NodeKind, languageKind, name, fqn, signature, parentID, contextID string, span Span) Node {
	stable := makeNodeStableKey(language, kind, fqn, name, signature, span)
	return Node{
		ID:                   stable,
		StableKey:            stable,
		Language:             language,
		Kind:                 kind,
		LanguageSpecificKind: languageKind,
		Name:                 name,
		FQN:                  fqn,
		Signature:            canonicalSignature(signature),
		ParentID:             parentID,
		ContextID:            contextID,
		Span:                 span,
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	b := [20]byte{}
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
