package symbolindex

type collector struct {
	symbolsByID map[string]*Symbol
	resultData  Result
}

func newCollector() *collector {
	return &collector{
		symbolsByID: make(map[string]*Symbol),
		resultData: Result{
			Symbols:    make([]Symbol, 0),
			Edges:      make([]Edge, 0),
			References: make([]Reference, 0),
			Calls:      make([]Call, 0),
		},
	}
}

func (c *collector) upsertSymbol(symbol Symbol) Symbol {
	if existing, ok := c.symbolsByID[symbol.ID]; ok {
		existing.Declarations = append(existing.Declarations, symbol.Declarations...)
		return *existing
	}
	copySymbol := symbol
	c.symbolsByID[symbol.ID] = &copySymbol
	c.resultData.Symbols = append(c.resultData.Symbols, copySymbol)
	return copySymbol
}

func (c *collector) addEdge(edge Edge) {
	c.resultData.Edges = append(c.resultData.Edges, edge)
}

func (c *collector) addReference(ref Reference) {
	c.resultData.References = append(c.resultData.References, ref)
}

func (c *collector) addCall(call Call) {
	c.resultData.Calls = append(c.resultData.Calls, call)
}

func (c *collector) findSymbolsByName(name string, contextID string) []Symbol {
	out := make([]Symbol, 0)
	for _, s := range c.resultData.Symbols {
		if s.Name == name && s.ContextID == contextID {
			out = append(out, s)
		}
	}
	return out
}

func (c *collector) result() Result {
	for i := range c.resultData.Symbols {
		if src, ok := c.symbolsByID[c.resultData.Symbols[i].ID]; ok {
			c.resultData.Symbols[i] = *src
		}
	}
	return c.resultData
}
