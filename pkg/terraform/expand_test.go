package terraform

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseBlocks(t *testing.T, src string) Blocks {
	t.Helper()
	f, diags := hclsyntax.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), diags.Error())
	content, diags := f.Body.Content(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "resource", LabelNames: []string{"type", "name"}},
			{Type: "data", LabelNames: []string{"type", "name"}},
		},
	})
	require.False(t, diags.HasErrors(), diags.Error())
	var blocks Blocks
	for _, hb := range content.Blocks {
		blocks = append(blocks, NewBlock(hb, nil, nil, nil, "", nil))
	}
	return blocks
}

func TestExpandBlocksCount(t *testing.T) {
	blocks := parseBlocks(t, `
resource "aws_s3_bucket" "a" { count = 3 }
resource "aws_s3_bucket" "b" { }
`)
	m := NewModule("", "", blocks, nil, true)
	out := m.ExpandBlocks()
	require.Len(t, out, 4)
	assert.Equal(t, "a[0]", out[0].Labels()[1])
	assert.Equal(t, "a[2]", out[2].Labels()[1])
	assert.Equal(t, "b", out[3].Labels()[1])
}

func TestExpandBlocksCountZero(t *testing.T) {
	blocks := parseBlocks(t, `resource "aws_s3_bucket" "a" { count = 0 }`)
	m := NewModule("", "", blocks, nil, true)
	assert.Empty(t, m.ExpandBlocks())
}

func TestExpandBlocksForEach(t *testing.T) {
	blocks := parseBlocks(t, `
resource "aws_s3_bucket" "a" {
  for_each = { x = "1", y = "2" }
}
`)
	m := NewModule("", "", blocks, nil, true)
	out := m.ExpandBlocks()
	require.Len(t, out, 2)
	assert.Contains(t, out[0].Labels()[1], "[")
}

func TestExpandDynamicBlocks(t *testing.T) {
	blocks := parseBlocks(t, `
resource "aws_security_group" "sg" {
  dynamic "ingress" {
    for_each = ["80", "443"]
    content {
      from_port = ingress.value
    }
  }
}
`)
	m := NewModule("", "", blocks, nil, true)
	out := m.ExpandBlocks()
	require.Len(t, out, 1)
	var ingress int
	for _, b := range out[0].AllBlocks() {
		if b.Type() == "ingress" {
			ingress++
		}
	}
	assert.Equal(t, 2, ingress)
}

func TestModulesExpandAll(t *testing.T) {
	blocks := parseBlocks(t, `resource "aws_s3_bucket" "a" { count = 2 }`)
	m := NewModule("", "", blocks, nil, true)
	mods := Modules{m}.ExpandAll()
	require.Len(t, mods[0].GetBlocks(), 2)
}

func TestUnusedResources(t *testing.T) {
	blocks := parseBlocks(t, `
resource "aws_s3_bucket" "used" { }
resource "aws_s3_bucket" "orphan" { }
resource "aws_instance" "vm" {
  ami = aws_s3_bucket.used.id
}
`)
	// parseBlocks schema only allows aws_s3_bucket-type resources? No —
	// resource blocks accept any type/name labels.
	m := NewModule("", "", blocks, nil, true)
	unused := Modules{m}.UnusedResources()
	// "orphan" and "vm" are unreferenced; "used" is referenced by vm.ami
	var names []string
	for _, b := range unused {
		names = append(names, b.NameLabel())
	}
	assert.ElementsMatch(t, []string{"orphan", "vm"}, names)
}

func TestRemoteStateBlocks(t *testing.T) {
	f, diags := hclsyntax.ParseConfig([]byte(`
data "terraform_remote_state" "prod" {
  backend = "s3"
  config = { bucket = "b" }
}
`), "test.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors())
	content, diags := f.Body.Content(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "data", LabelNames: []string{"type", "name"}}},
	})
	require.False(t, diags.HasErrors())
	var blocks Blocks
	for _, hb := range content.Blocks {
		blocks = append(blocks, NewBlock(hb, nil, nil, nil, "", nil))
	}
	m := NewModule("", "", blocks, nil, true)
	assert.Len(t, Modules{m}.RemoteStateBlocks(), 1)
}
