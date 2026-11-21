package terraform

// UnusedResources returns resource blocks that no other block references.
// It complements the spec-gap coverage report: a resource that exists but
// is never referenced is dead configuration (or a missed dependency edge).
// References are matched via the same Reference.RefersTo logic used by
// GetReferencingBlocks, so count/for_each keys are handled.
func (m Modules) UnusedResources() Blocks {
	// Collect every attribute reference and resource block in the set once.
	var refs []*Reference
	var resBlocks Blocks
	for _, mod := range m {
		if mod == nil {
			continue
		}
		for _, b := range mod.GetBlocks() {
			collectReferences(b, &refs)
			if b.Type() == "resource" {
				resBlocks = append(resBlocks, b)
			}
		}
	}

	var unused Blocks
	for _, res := range resBlocks {
		used := false
		for _, ref := range refs {
			if ref != nil && ref.RefersTo(res.Reference()) {
				used = true
				break
			}
		}
		if !used {
			unused = append(unused, res)
		}
	}
	return unused
}

func collectReferences(b *Block, refs *[]*Reference) {
	for _, attr := range b.GetAttributes() {
		*refs = append(*refs, attr.AllReferences()...)
	}
	for _, child := range b.AllBlocks() {
		collectReferences(child, refs)
	}
}

// RemoteStateBlocks returns data "terraform_remote_state" blocks — these
// read state produced elsewhere, so resources they expose cannot be
// resolved statically. Reported as analysis gaps rather than failures.
func (m Modules) RemoteStateBlocks() Blocks {
	var out Blocks
	for _, mod := range m {
		if mod == nil {
			continue
		}
		out = append(out, mod.GetDatasByType("terraform_remote_state")...)
	}
	return out
}
