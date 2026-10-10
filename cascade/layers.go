package cascade

// Layers is a tree of cascade layers (CSS Cascade 5 §6.4,
// https://www.w3.org/TR/css-cascade-5/#layering), in the order they are
// first declared. Layer 0 is the implicit outer layer, holding the
// unlayered rules. The zero value holds only that layer.
//
// Declaring layers modifies a Layers; the other methods only read it, so
// that the Layers of a shared stylesheet can be used concurrently. A Layers
// must not be copied once it has layers.
type Layers struct {
	nodes []layerNode      // indexed by layer id; nodes[0] is the outer layer
	named map[layerKey]int // the named children, by parent and name
}

type layerKey struct {
	parent int
	name   string
}

type layerNode struct {
	name      string // one segment of the layer name; "" for the outer layer
	anonymous bool
	parent    int
	children  []int // in order of first declaration
}

// init materializes the outer layer, implicit until the first declaration.
func (l *Layers) init() {
	if len(l.nodes) == 0 {
		l.nodes = []layerNode{{parent: -1}}
	}
}

// Len returns the number of layers, the outer layer included.
func (l *Layers) Len() int { return max(1, len(l.nodes)) }

// Declare returns the layer named by the segments of name (for
// "framework.theme", ["framework", "theme"]) nested in parent, declaring
// it and its missing ancestors if this is their first declaration.
func (l *Layers) Declare(parent int, name []string) int {
	l.init()
	id := parent
	for _, segment := range name {
		id = l.child(id, segment)
	}
	return id
}

// child returns the named child of parent, declaring it if needed.
func (l *Layers) child(parent int, segment string) int {
	key := layerKey{parent, segment}
	if c, ok := l.named[key]; ok {
		return c
	}
	if l.named == nil {
		l.named = map[layerKey]int{}
	}
	c := l.add(layerNode{name: segment, parent: parent})
	l.named[key] = c
	return c
}

// Anonymous declares a new anonymous layer nested in parent: every call
// declares a distinct layer.
func (l *Layers) Anonymous(parent int) int {
	l.init()
	return l.add(layerNode{anonymous: true, parent: parent})
}

func (l *Layers) add(node layerNode) int {
	id := len(l.nodes)
	l.nodes = append(l.nodes, node)
	l.nodes[node.parent].children = append(l.nodes[node.parent].children, id)
	return id
}

// Name returns the full name of the layer, its segments from the outer
// layer; an anonymous segment is "".
func (l *Layers) Name(id int) []string {
	var name []string
	for ; id > 0; id = l.nodes[id].parent {
		name = append([]string{l.nodes[id].name}, name...)
	}
	return name
}

// Import declares the layers of other into l, nested in parent, in
// other's order of declaration: named layers merge with those of the
// same name, anonymous ones stay distinct. It returns, for each layer of
// other, the corresponding layer of l. Use it to combine the layers of
// the stylesheets of one origin, in document order.
func (l *Layers) Import(other *Layers, parent int) []int {
	l.init()
	mapping := make([]int, other.Len())
	mapping[0] = parent
	// ids grow in order of declaration, and parents are declared first
	for id := 1; id < len(other.nodes); id++ {
		node := other.nodes[id]
		if node.anonymous {
			mapping[id] = l.Anonymous(mapping[node.parent])
		} else {
			mapping[id] = l.child(mapping[node.parent], node.name)
		}
	}
	return mapping
}

// Ranks returns the rank of each layer in the layer order, indexed by
// layer id: layers are sorted by first declaration, nested layers within
// their parent, and the rules directly in a layer come after its nested
// layers. The outer layer (unlayered rules) thus ranks last.
//
// For normal declarations a higher rank wins; for !important ones, a
// lower rank.
func (l *Layers) Ranks() []int {
	if len(l.nodes) == 0 {
		return []int{0}
	}
	ranks := make([]int, len(l.nodes))
	next := 0
	var visit func(id int)
	visit = func(id int) {
		for _, c := range l.nodes[id].children {
			visit(c)
		}
		ranks[id] = next
		next++
	}
	visit(0)
	return ranks
}
