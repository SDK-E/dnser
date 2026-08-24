package generator

import (
	"fmt"
)

func DeepMerge(dst, src map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(dst)+len(src))
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		existing, ok := out[k]
		if !ok {
			out[k] = v
			continue
		}
		merged, err := mergeValues(existing, v)
		if err != nil {
			return nil, fmt.Errorf("merge key %q: %w", k, err)
		}
		out[k] = merged
	}
	return out, nil
}

func mergeValues(dstVal, srcVal any) (any, error) {
	dstMap, dstIsMap := dstVal.(map[string]any)
	srcMap, srcIsMap := srcVal.(map[string]any)
	if dstIsMap && srcIsMap {
		return DeepMerge(dstMap, srcMap)
	}
	return srcVal, nil
}
