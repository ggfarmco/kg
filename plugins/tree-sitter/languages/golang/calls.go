package golang

import (
	"context"

	sitter "github.com/smacker/go-tree-sitter"
)

func walkCalls(g *GoLang, fs FileSource, cs CallSink) error {
	tree, err := g.parse(context.Background(), fs.Bytes())
	if err != nil {
		return err
	}
	defer tree.Close()
	root := tree.RootNode()
	src := fs.Bytes()
	pkgSlug := fs.PackageSlug()
	fileSlug := fs.FileSlug()

	for i := 0; i < int(root.NamedChildCount()); i++ {
		fn := root.NamedChild(i)
		var fnName string
		switch fn.Type() {
		case "function_declaration", "method_declaration":
			name := fn.ChildByFieldName("name")
			if name == nil {
				continue
			}
			fnName = sanitizeIdent(name.Content(src))
		default:
			continue
		}
		body := fn.ChildByFieldName("body")
		if body == nil {
			continue
		}
		walkCallExprs(body, src, func(callee string) {
			if !cs.HasDeclInFile(fileSlug, callee) {
				return
			}
			from := pkgSlug + "/" + fileSlug + "::" + fnName
			to := pkgSlug + "/" + fileSlug + "::" + callee
			cs.AddCall(from, to)
		})
	}
	return nil
}

func walkCallExprs(n *sitter.Node, src []byte, onCall func(string)) {
	if n.Type() == "call_expression" {
		fn := n.ChildByFieldName("function")
		if fn != nil && fn.Type() == "identifier" {
			onCall(sanitizeIdent(fn.Content(src)))
		}
	}
	for i := 0; i < int(n.NamedChildCount()); i++ {
		walkCallExprs(n.NamedChild(i), src, onCall)
	}
}
