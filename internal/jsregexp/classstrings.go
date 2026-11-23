package jsregexp

import (
	"cmp"
	"slices"
)

// stringTrie indexes the non-empty strings of a v-mode class by their code points
// in matching order, reversed inside a lookbehind, so that a single pass over the
// input finds every string of the class that matches at a position.
type stringTrie struct {
	nodes []trieNode // nodes[0] is the empty prefix
}

// trieNode is a prefix of the strings: edges, sorted by code point, lead to the
// prefixes one code point longer, and end reports that the prefix is a string.
type trieNode struct {
	edges []trieEdge
	end   bool
}

type trieEdge struct {
	c    rune
	node int
}

func compareEdge(e trieEdge, c rune) int { return cmp.Compare(e.c, c) }

// newStringTrie indexes strs, each read from its end when reversed is set.
func newStringTrie(strs [][]rune, reversed bool) *stringTrie {
	t := &stringTrie{nodes: make([]trieNode, 1)}
	for _, s := range strs {
		n := 0
		for i := range s {
			c := s[i]
			if reversed {
				c = s[len(s)-1-i]
			}
			n = t.child(n, c)
		}
		t.nodes[n].end = true
	}
	return t
}

// child returns the node reached from n by c, adding it if needed.
func (t *stringTrie) child(n int, c rune) int {
	i, ok := slices.BinarySearchFunc(t.nodes[n].edges, c, compareEdge)
	if ok {
		return t.nodes[n].edges[i].node
	}
	t.nodes = append(t.nodes, trieNode{})
	next := len(t.nodes) - 1
	t.nodes[n].edges = slices.Insert(t.nodes[n].edges, i, trieEdge{c, next})
	return next
}

// next returns the node reached from n by c, or -1 when no string continues
// with c.
func (t *stringTrie) next(n int, c rune) int {
	edges := t.nodes[n].edges
	if i, ok := slices.BinarySearchFunc(edges, c, compareEdge); ok {
		return edges[i].node
	}
	return -1
}
