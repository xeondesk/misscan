package terraform

import (
	"math/big"

	"github.com/zclconf/go-cty/cty"
)

// ExpandBlocks returns the module's blocks with `count` and `for_each`
// meta-arguments expanded into concrete clones. Blocks that cannot be
// statically expanded (non-literal count/for_each, dynamic blocks) are
// kept as-is so checks still see them — expansion is best-effort and
// never drops a block.
//
// Expanded clones carry count.index / each.key / each.value in their
// evaluation context and suffixed labels (e.g. aws_s3_bucket.b[0]).
func (m *Module) ExpandBlocks() Blocks {
	var out Blocks
	for _, b := range m.blocks {
		for _, eb := range expandBlock(b) {
			expandDynamicChildren(eb)
			out = append(out, eb)
		}
	}
	m.blocks = out
	m.rebuildBlockMap()
	return out
}

// rebuildBlockMap refreshes the type-label index after expansion changed
// block names (e.g. aws_s3_bucket.a[0]).
func (m *Module) rebuildBlockMap() {
	m.blockMap = make(map[string]Blocks)
	for _, b := range m.blocks {
		if b.NameLabel() != "" {
			m.blockMap[b.TypeLabel()] = append(m.blockMap[b.TypeLabel()], b)
		}
	}
}

// expandDynamicChildren replaces `dynamic "x" { for_each = ...; content {} }`
// blocks with one concrete child block per element. Unresolvable dynamics
// are left in place.
func expandDynamicChildren(b *Block) {
	var kept, generated Blocks
	for _, child := range b.AllBlocks() {
		if child.Type() != "dynamic" {
			kept = append(kept, child)
			continue
		}
		var label string
		if lbs := child.Labels(); len(lbs) > 0 {
			label = lbs[0]
		}
		fe := child.GetAttribute("for_each")
		content := child.GetBlock("content")
		if label == "" || fe == nil || content == nil || !fe.IsResolvable() {
			kept = append(kept, child)
			continue
		}
		var made bool
		fe.Each(func(key, val cty.Value) {
			made = true
			c := content.Clone(key)
			c.hclBlock.Type = label
			c.hclBlock.Labels = nil
			c.Context().SetByDot(key, "each.key")
			c.Context().SetByDot(val, "each.value")
			generated = append(generated, c)
		})
		if !made {
			kept = append(kept, child)
		}
	}
	b.childBlocks = append(kept, generated...)
}

func expandBlock(b *Block) Blocks {
	if b.IsCountExpanded() {
		return Blocks{b}
	}

	if countAttr := b.GetAttribute("count"); countAttr != nil && countAttr.IsResolvable() {
		if n, ok := countValue(countAttr); ok {
			if n <= 0 {
				return nil // count = 0 removes the block
			}
			clones := make(Blocks, 0, n)
			for i := 0; i < n; i++ {
				clones = append(clones, b.Clone(cty.NumberIntVal(int64(i))))
			}
			return clones
		}
	}

	if feAttr := b.GetAttribute("for_each"); feAttr != nil && feAttr.IsResolvable() {
		var clones Blocks
		feAttr.Each(func(key, val cty.Value) {
			var idx cty.Value
			if key.Type() == cty.String {
				idx = key
			} else {
				idx = cty.NumberIntVal(0)
			}
			clone := b.Clone(idx)
			clone.Context().SetByDot(key, "each.key")
			clone.Context().SetByDot(val, "each.value")
			clones = append(clones, clone)
		})
		if len(clones) > 0 {
			return clones
		}
	}

	return Blocks{b}
}

func countValue(attr *Attribute) (int, bool) {
	v := attr.Value()
	if !v.IsKnown() || v.IsNull() || v.Type() != cty.Number {
		return 0, false
	}
	bf := v.AsBigFloat()
	i, acc := bf.Int64()
	if acc != big.Exact || i < 0 {
		return 0, false
	}
	return int(i), true
}

// ExpandAll expands count/for_each/dynamic blocks across all modules.
func (m Modules) ExpandAll() Modules {
	out := make(Modules, 0, len(m))
	for _, mod := range m {
		if mod == nil {
			continue
		}
		mod.ExpandBlocks()
		out = append(out, mod)
	}
	return out
}
