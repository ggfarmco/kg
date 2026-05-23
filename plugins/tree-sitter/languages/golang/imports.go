package golang

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

func walkImports(root *sitter.Node, src []byte, is ImportSink) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child.Type() != "import_declaration" {
			continue
		}
		for j := 0; j < int(child.NamedChildCount()); j++ {
			inner := child.NamedChild(j)
			switch inner.Type() {
			case "import_spec":
				addImportSpec(inner, src, is)
			case "import_spec_list":
				for k := 0; k < int(inner.NamedChildCount()); k++ {
					spec := inner.NamedChild(k)
					if spec.Type() == "import_spec" {
						addImportSpec(spec, src, is)
					}
				}
			}
		}
	}
}

func addImportSpec(spec *sitter.Node, src []byte, is ImportSink) {
	path := spec.ChildByFieldName("path")
	if path == nil {
		return
	}
	raw := path.Content(src)
	raw = strings.Trim(raw, "`\"")
	is.AddImport(raw)
}
