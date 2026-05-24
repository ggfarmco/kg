package snapshot

import "fmt"

func TopoSortNodes(specs []NodeSpec) ([]NodeSpec, error) {
	idx := make(map[string]int, len(specs))
	for i, n := range specs {
		idx[n.ID] = i
	}
	color := make([]int, len(specs))
	out := make([]NodeSpec, 0, len(specs))

	var visit func(int) error
	visit = func(i int) error {
		switch color[i] {
		case 1:
			return fmt.Errorf("cycle through %q", specs[i].ID)
		case 2:
			return nil
		}
		color[i] = 1
		parent := specs[i].Parent
		if parent != "" {
			if pi, ok := idx[parent]; ok {
				if err := visit(pi); err != nil {
					return err
				}
			}
		}
		color[i] = 2
		out = append(out, specs[i])
		return nil
	}

	for i := range specs {
		if err := visit(i); err != nil {
			return nil, err
		}
	}
	return out, nil
}
