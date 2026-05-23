package golang

import (
	sitter "github.com/smacker/go-tree-sitter"
)

func walkDecls(root *sitter.Node, src []byte, ds DeclSink) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		switch child.Type() {
		case "function_declaration":
			emitFunc(child, src, ds, false)
		case "method_declaration":
			emitFunc(child, src, ds, true)
		case "type_declaration":
			emitType(child, src, ds)
		case "var_declaration":
			emitVarConst(child, src, ds, "var")
		case "const_declaration":
			emitVarConst(child, src, ds, "const")
		}
	}
}

func emitFunc(n *sitter.Node, src []byte, ds DeclSink, isMethod bool) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(src)
	props := map[string]any{
		"kind":       "function",
		"name":       name,
		"exported":   IsExported(name),
		"line_start": int(n.StartPoint().Row) + 1,
		"line_end":   int(n.EndPoint().Row) + 1,
		"params":     extractParams(n.ChildByFieldName("parameters"), src),
		"returns":    nodeText(n.ChildByFieldName("result"), src),
	}
	if isMethod {
		props["kind"] = "method"
		props["receiver"] = extractReceiver(n.ChildByFieldName("receiver"), src)
	}
	ds.AddDecl(sanitizeIdent(name), props)
}

func emitType(n *sitter.Node, src []byte, ds DeclSink) {
	for i := 0; i < int(n.NamedChildCount()); i++ {
		spec := n.NamedChild(i)
		if spec.Type() != "type_spec" {
			continue
		}
		nameNode := spec.ChildByFieldName("name")
		typeNode := spec.ChildByFieldName("type")
		if nameNode == nil || typeNode == nil {
			continue
		}
		name := nameNode.Content(src)
		kind := "type"
		switch typeNode.Type() {
		case "struct_type":
			kind = "struct"
		case "interface_type":
			kind = "interface"
		}
		ds.AddDecl(sanitizeIdent(name), map[string]any{
			"kind":       kind,
			"name":       name,
			"exported":   IsExported(name),
			"line_start": int(n.StartPoint().Row) + 1,
			"line_end":   int(n.EndPoint().Row) + 1,
		})
	}
}

func emitVarConst(n *sitter.Node, src []byte, ds DeclSink, kind string) {
	for i := 0; i < int(n.NamedChildCount()); i++ {
		spec := n.NamedChild(i)
		nameNode := spec.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nameNode.Content(src)
		ds.AddDecl(sanitizeIdent(name), map[string]any{
			"kind":       kind,
			"name":       name,
			"exported":   IsExported(name),
			"line_start": int(spec.StartPoint().Row) + 1,
			"line_end":   int(spec.EndPoint().Row) + 1,
		})
	}
}

func extractParams(params *sitter.Node, src []byte) []string {
	if params == nil {
		return nil
	}
	var out []string
	for i := 0; i < int(params.NamedChildCount()); i++ {
		decl := params.NamedChild(i)
		if decl.Type() != "parameter_declaration" {
			continue
		}
		for j := 0; j < int(decl.NamedChildCount()); j++ {
			child := decl.NamedChild(j)
			if child.Type() == "identifier" {
				out = append(out, child.Content(src))
			}
		}
	}
	return out
}

func extractReceiver(recv *sitter.Node, src []byte) string {
	if recv == nil {
		return ""
	}
	for i := 0; i < int(recv.NamedChildCount()); i++ {
		decl := recv.NamedChild(i)
		if decl.Type() != "parameter_declaration" {
			continue
		}
		for j := 0; j < int(decl.NamedChildCount()); j++ {
			child := decl.NamedChild(j)
			if child.Type() == "type_identifier" {
				return child.Content(src)
			}
			if child.Type() == "pointer_type" {
				for k := 0; k < int(child.NamedChildCount()); k++ {
					inner := child.NamedChild(k)
					if inner.Type() == "type_identifier" {
						return inner.Content(src)
					}
				}
			}
		}
	}
	return ""
}

func nodeText(n *sitter.Node, src []byte) string {
	if n == nil {
		return ""
	}
	return n.Content(src)
}

func sanitizeIdent(name string) string {
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z':
			out = append(out, c+32)
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			out = append(out, c)
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}
