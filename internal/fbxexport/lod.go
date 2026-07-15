package fbxexport

import (
	"errors"

	"github.com/jmhobbs/odol/internal/model"
)

var errNoLODs = errors.New("model has no LODs")

func selectLOD(m *model.Model) (*model.LOD, error) {
	if len(m.LODs) == 0 {
		return nil, errNoLODs
	}
	best := 0
	for i := 1; i < len(m.LODs); i++ {
		if m.LODs[i].Resolution < m.LODs[best].Resolution {
			best = i
		}
	}
	return &m.LODs[best], nil
}
